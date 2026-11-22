package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/storage"
)

var filename = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`)

type SessionStore interface {
	ListSessionPluginViews() ([]storage.SessionPluginView, error)
}

type PageContent struct {
	View  string     `json:"view"`
	Items []PageItem `json:"items"`
}

type PageItem struct {
	Folder  string       `json:"folder"`
	Data    any          `json:"data"`
	Threads []PageThread `json:"threads"`
}

type PageThread struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) Page(plugin string, sessions SessionStore) (PageContent, error) {
	return s.PageWithContext(context.Background(), plugin, sessions)
}

func (s *Store) PageWithContext(ctx context.Context, plugin string, sessions SessionStore) (PageContent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !manifest.SafeName(plugin) {
		return PageContent{}, fmt.Errorf("%w: name %q", ErrInvalid, plugin)
	}
	installed, err := manifest.Read(filepath.Join(s.dir, plugin))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, manifest.ErrNotPlugin) {
		return PageContent{}, ErrNotFound
	}
	if err != nil {
		return PageContent{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if installed.Name != plugin || installed.Page == nil {
		return PageContent{}, ErrNotFound
	}
	html, err := s.view(plugin + "/" + installed.Page.View)
	if err != nil {
		return PageContent{}, err
	}
	page := PageContent{View: html, Items: []PageItem{}}
	seenThreads := map[string]bool{}

	if s.stateStore != nil {
		rows, err := s.stateStore.ListThreadPluginStatesByPlugin(ctx, plugin)
		if err != nil {
			return PageContent{}, err
		}
		for _, row := range rows {
			if seenThreads[row.ThreadID] {
				continue
			}
			var value any
			if len(row.DataJSON) == 0 || string(row.DataJSON) == "null" || json.Unmarshal(row.DataJSON, &value) != nil || value == nil {
				continue
			}
			seenThreads[row.ThreadID] = true
			page.Items = append(page.Items, PageItem{
				Folder: row.ThreadCwd,
				Data:   value,
				Threads: []PageThread{{
					ID:        row.ThreadID,
					Title:     row.ThreadTitle,
					Status:    row.ThreadStatus,
					UpdatedAt: row.ThreadUpdatedAt,
				}},
			})
		}
	}

	if sessions == nil {
		return page, nil
	}
	presentations, err := sessions.ListSessionPluginViews()
	if err != nil {
		return PageContent{}, err
	}
	latest := map[string]storage.SessionPluginView{}
	for _, presentation := range presentations {
		if seenThreads[presentation.ID] {
			continue
		}
		owner, view, ok := strings.Cut(presentation.Reference, "/")
		if !ok || owner != plugin || !name.MatchString(view) || presentation.File == "" {
			continue
		}
		if previous, found := latest[presentation.ID]; !found || presentation.Seq > previous.Seq {
			latest[presentation.ID] = presentation
		}
	}
	threads := make([]storage.SessionPluginView, 0, len(latest))
	for _, thread := range latest {
		threads = append(threads, thread)
	}
	slices.SortFunc(threads, func(a, b storage.SessionPluginView) int {
		if order := b.UpdatedAt.Compare(a.UpdatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	type source struct {
		folder string
		file   string
	}
	groups := map[source][]PageThread{}
	var ordered []source
	for _, thread := range threads {
		if !filepath.IsAbs(thread.Folder) || !filename.MatchString(thread.File) {
			continue
		}
		key := source{folder: filepath.Clean(thread.Folder), file: thread.File}
		if _, found := groups[key]; !found {
			ordered = append(ordered, key)
		}
		groups[key] = append(groups[key], PageThread{ID: thread.ID, Title: thread.Title, Status: thread.Status, UpdatedAt: thread.UpdatedAt})
	}
	for _, key := range ordered {
		root, err := os.OpenRoot(key.folder)
		if err != nil {
			continue
		}
		data, err := root.ReadFile(key.file)
		root.Close()
		var value any
		if err != nil || json.Unmarshal(data, &value) != nil {
			continue
		}
		page.Items = append(page.Items, PageItem{Folder: key.folder, Data: value, Threads: groups[key]})
	}
	return page, nil
}
