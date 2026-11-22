package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type SourceView struct {
	ID            string             `json:"id"`
	Repository    string             `json:"repository"`
	URL           string             `json:"url"`
	Ref           string             `json:"ref,omitempty"`
	Paths         []string           `json:"paths,omitempty"`
	CommitSHA     string             `json:"commit_sha,omitempty"`
	AutoUpdate    bool               `json:"auto_update,omitempty"`
	IsMarketplace bool               `json:"is_marketplace,omitempty"`
	BuiltIn       bool               `json:"built_in,omitempty"`
	Plugins       []MarketplaceEntry `json:"plugins,omitempty"`
	CheckedAt     time.Time          `json:"checked_at,omitempty"`
}

func DefaultMarketplaceSourceID() string {
	repoURL, err := repositoryURL(DefaultMarketplaceRepo)
	if err != nil {
		return strings.ToLower(DefaultMarketplaceRepo)
	}
	return sourceIDFor(repoURL, "")
}

func (s *Service) ListSources(ctx context.Context) ([]SourceView, error) {
	if s.state == nil {
		return []SourceView{}, nil
	}
	records, err := s.state.ListPluginSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SourceView, 0, len(records))
	for _, record := range records {
		var entries []MarketplaceEntry
		_ = json.Unmarshal(record.CatalogJSON, &entries)
		out = append(out, sourceViewFromRecord(record, entries))
	}
	return out, nil
}

func (s *Service) AddSource(ctx context.Context, input RepositoryInput) (SourceView, error) {
	if s.store == nil || s.state == nil {
		return SourceView{}, errors.New("plugin store unavailable")
	}
	record, entries, err := s.store.InspectSource(ctx, input)
	if err != nil {
		return SourceView{}, err
	}
	if err := s.state.SavePluginSource(ctx, record); err != nil {
		return SourceView{}, err
	}
	return sourceViewFromRecord(record, entries), nil
}

// EnsureDefaultSource seeds the official Jaz plugin marketplace when it is not
// already registered.
func (s *Service) EnsureDefaultSource(ctx context.Context) (bool, error) {
	if s.store == nil || s.state == nil {
		return false, nil
	}
	sources, err := s.state.ListPluginSources(ctx)
	if err != nil {
		return false, err
	}
	officialID := DefaultMarketplaceSourceID()
	for _, src := range sources {
		if src.ID == officialID {
			return false, nil
		}
	}
	if _, err := s.AddSource(ctx, RepositoryInput{
		Repository: DefaultMarketplaceRepo,
		AutoUpdate: true,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// SyncSources ensures the official marketplace is seeded and refreshes all
// configured sources and installed Git plugins without redundantly re-checking
// a freshly seeded official catalog in the same pass.
func (s *Service) SyncSources(ctx context.Context) error {
	added, ensureErr := s.EnsureDefaultSource(ctx)
	skipID := ""
	if added {
		skipID = DefaultMarketplaceSourceID()
	}
	return errors.Join(ensureErr, s.refreshSources(ctx, skipID))
}

func (s *Service) RemoveSource(ctx context.Context, id string) error {
	if s.state == nil {
		return ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == DefaultMarketplaceSourceID() {
		return fmt.Errorf("%w: the official Jaz marketplace cannot be removed", ErrInvalid)
	}
	deleted, err := s.state.DeletePluginSource(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

// RefreshSources checks configured marketplaces and installed Git plugins via
// deduplicated parallel git ls-remote calls, updating cached catalogs and
// auto-updating sources marked auto_update=true.
func (s *Service) RefreshSources(ctx context.Context) error {
	return s.refreshSources(ctx, "")
}

func (s *Service) refreshSources(ctx context.Context, skipSourceID string) error {
	if s.store == nil || s.state == nil {
		return nil
	}
	sources, err := s.state.ListPluginSources(ctx)
	if err != nil {
		return err
	}
	states, err := s.state.ListPluginStates(ctx)
	if err != nil {
		return err
	}

	type repoRef struct{ url, ref string }
	type remoteResult struct {
		sha string
		err error
	}
	targets := map[repoRef]struct{}{}
	for _, src := range sources {
		if src.ID == skipSourceID {
			continue
		}
		targets[repoRef{url: src.URL, ref: src.Ref}] = struct{}{}
	}
	stateURLs := make(map[string]string, len(states))
	for _, st := range states {
		if st.SourceRepository == "" || commitID.MatchString(st.SourceRef) {
			continue
		}
		if repoURL, err := repositoryURL(st.SourceRepository); err == nil {
			stateURLs[st.PluginID] = repoURL
			targets[repoRef{url: repoURL, ref: st.SourceRef}] = struct{}{}
		}
	}

	results := make(map[repoRef]remoteResult, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for target := range targets {
		wg.Add(1)
		go func(t repoRef) {
			defer wg.Done()
			sha, err := RemoteCommit(ctx, t.url, t.ref)
			mu.Lock()
			results[t] = remoteResult{sha: sha, err: err}
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	var errs []error
	autoSources := make(map[string]bool, len(sources))
	for _, src := range sources {
		autoSources[src.ID] = src.AutoUpdate
		if src.ID == skipSourceID {
			continue
		}
		res := results[repoRef{url: src.URL, ref: src.Ref}]
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		if res.sha != src.CommitSHA {
			updated, _, inspectErr := s.store.InspectSource(ctx, RepositoryInput{
				Repository: src.Repository,
				Ref:        src.Ref,
				Paths:      src.Paths,
				AutoUpdate: src.AutoUpdate,
			})
			if inspectErr != nil {
				errs = append(errs, inspectErr)
				continue
			}
			src = updated
		} else {
			src.CheckedAt = time.Now().UTC()
		}
		if err := s.state.SavePluginSource(ctx, src); err != nil {
			errs = append(errs, err)
		}
	}
	for _, st := range states {
		repoURL, ok := stateURLs[st.PluginID]
		if !ok {
			continue
		}
		res := results[repoRef{url: repoURL, ref: st.SourceRef}]
		if res.err != nil {
			continue
		}
		if res.sha != st.CommitSHA && autoSources[st.SourceID] {
			if _, updateErr := s.store.Update(ctx, st.PluginID); updateErr != nil {
				errs = append(errs, updateErr)
			}
			continue
		}
		if res.sha != st.RemoteSHA {
			st.RemoteSHA = res.sha
			if saveErr := s.state.SavePluginState(ctx, st); saveErr != nil {
				errs = append(errs, saveErr)
			}
		}
	}
	return errors.Join(errs...)
}

func sourceViewFromRecord(record storage.PluginSourceRecord, entries []MarketplaceEntry) SourceView {
	return SourceView{
		ID:            record.ID,
		Repository:    record.Repository,
		URL:           record.URL,
		Ref:           record.Ref,
		Paths:         record.Paths,
		CommitSHA:     record.CommitSHA,
		AutoUpdate:    record.AutoUpdate,
		IsMarketplace: record.IsMarketplace,
		BuiltIn:       record.ID == DefaultMarketplaceSourceID(),
		Plugins:       entries,
		CheckedAt:     record.CheckedAt,
	}
}
