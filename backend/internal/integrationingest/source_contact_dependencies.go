package integrationingest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const sourceKindContactDependency = "contact_dependency"

type contactDependencyAddress struct {
	Provider string
	Account  string
	Slug     string
}

type contactDependencyEntry struct {
	Path      string                 `json:"path"`
	Provider  string                 `json:"provider,omitempty"`
	Kind      string                 `json:"kind,omitempty"`
	MediaType string                 `json:"media_type,omitempty"`
	Key       integrations.SourceKey `json:"key,omitempty"`
	Replay    integrations.Replay    `json:"replay,omitempty"`
}

func (e contactDependencyEntry) MarshalJSON() ([]byte, error) {
	var out struct {
		Path      string                  `json:"path"`
		Provider  string                  `json:"provider,omitempty"`
		Kind      string                  `json:"kind,omitempty"`
		MediaType string                  `json:"media_type,omitempty"`
		Key       *integrations.SourceKey `json:"key,omitempty"`
		Replay    *integrations.Replay    `json:"replay,omitempty"`
	}
	out.Path = e.Path
	out.Provider = e.Provider
	out.Kind = e.Kind
	out.MediaType = e.MediaType
	if !e.Key.IsZero() {
		key := e.Key
		out.Key = &key
	}
	if !e.Replay.IsZero() {
		replay := e.Replay
		out.Replay = &replay
	}
	return json.Marshal(out)
}

func (e *contactDependencyEntry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Path         string                 `json:"path"`
		Provider     string                 `json:"provider,omitempty"`
		Kind         string                 `json:"kind,omitempty"`
		MediaType    string                 `json:"media_type,omitempty"`
		Key          integrations.SourceKey `json:"key,omitempty"`
		Replay       integrations.Replay    `json:"replay,omitempty"`
		LegacyEntity string                 `json:"source_id,omitempty"`
		LegacyDay    string                 `json:"day,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Path = raw.Path
	e.Provider = raw.Provider
	e.Kind = raw.Kind
	e.MediaType = raw.MediaType
	e.Key = sourceKeyWithLegacy(raw.Key, raw.LegacyEntity, raw.LegacyDay)
	e.Replay = raw.Replay
	return nil
}

func sourceKeyWithLegacy(key integrations.SourceKey, entity, day string) integrations.SourceKey {
	if key != (integrations.SourceKey{}) {
		return key
	}
	return integrations.SourceKey{Entity: entity, Day: day}
}

func contactDependencySource(record integrations.Record) (sourcequeue.Source, bool) {
	provider := integrations.NormalizeAlias(record.Provider)
	account := integrations.NormalizeAlias(record.AccountID)
	if provider == "" || account == "" || strings.TrimSpace(record.ExternalID) == "" {
		return sourcequeue.Source{}, false
	}
	path := filepath.ToSlash(filepath.Join(".state", "source-dependencies", "chat-contact", provider, account, integrations.SourceSlug(record.ExternalID)+".dep"))
	return sourcequeue.Source{Path: path, PendingAt: recordTime(record), Provider: provider, Kind: sourceKindContactDependency}, true
}

func (p SourceProjector) projectContactDependency(ctx context.Context, dependency contactDependencyAddress, pendingAt time.Time) ([]integrations.Artifact, error) {
	pending, err := p.contactDependencySources(dependency, pendingAt)
	if err != nil {
		return nil, err
	}
	var artifacts []integrations.Artifact
	for _, source := range pending {
		projected, err := p.ProjectSource(ctx, source)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, projected...)
	}
	return artifacts, nil
}

func (p SourceProjector) recordSourceDependencies(target integrations.SourceTarget) error {
	if strings.TrimSpace(p.StateRoot) == "" || len(target.ContactRefs) == 0 {
		return nil
	}
	entry := contactDependencyEntry{
		Path:      target.PathHint,
		Provider:  target.Provider,
		Kind:      target.Kind,
		MediaType: target.MediaType,
		Key:       target.Key,
		Replay:    target.Replay,
	}
	for _, ref := range target.ContactRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		if err := p.recordContactDependency(target.Provider, target.Replay.Account, integrations.SourceSlug(ref), entry); err != nil {
			return err
		}
	}
	return nil
}

func (p SourceProjector) recordContactDependency(provider, account, contactSlug string, entry contactDependencyEntry) error {
	path, err := p.contactDependencyIndexPath(contactDependencyAddress{Provider: provider, Account: account, Slug: contactSlug})
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	index, err := readContactDependencyIndex(path)
	if err != nil {
		return err
	}
	if index == nil {
		index = map[string]contactDependencyEntry{}
	}
	index[entry.Path] = entry
	return writeContactDependencyIndex(path, index)
}

func (p SourceProjector) contactDependencySources(dependency contactDependencyAddress, pendingAt time.Time) ([]sourcequeue.Source, error) {
	path, err := p.contactDependencyIndexPath(dependency)
	if err != nil {
		return nil, err
	}
	index, err := readContactDependencyIndex(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]sourcequeue.Source{}
	for _, entry := range index {
		sourcePath, err := cleanSourcePath(entry.Path)
		if err != nil {
			return nil, err
		}
		seen[sourcePath] = sourcequeue.Source{
			Path:      sourcePath,
			PendingAt: pendingAt,
			Provider:  firstNonEmpty(entry.Provider, dependency.Provider),
			Kind:      entry.Kind,
			MediaType: entry.MediaType,
			Key:       entry.Key,
			Replay:    entry.Replay,
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]sourcequeue.Source, 0, len(paths))
	for _, path := range paths {
		out = append(out, seen[path])
	}
	return out, nil
}

func (p SourceProjector) contactDependencyIndexPath(dependency contactDependencyAddress) (string, error) {
	root, err := requiredStateRoot(p.StateRoot)
	if err != nil {
		return "", err
	}
	provider, err := requiredPathComponent("provider", dependency.Provider)
	if err != nil {
		return "", err
	}
	account, err := requiredPathComponent("account", dependency.Account)
	if err != nil {
		return "", err
	}
	contact, err := requiredPathComponent("contact", dependency.Slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".state", "source-dependencies", "chat-contact", provider, account, contact+".json"), nil
}

func parseContactDependencyPath(sourcePath string) (contactDependencyAddress, error) {
	path, err := cleanSourcePath(sourcePath)
	if err != nil {
		return contactDependencyAddress{}, err
	}
	parts := strings.Split(path, "/")
	if len(parts) == 6 && parts[0] == ".state" && parts[1] == "source-dependencies" && parts[2] == "chat-contact" && strings.HasSuffix(parts[5], ".dep") {
		return contactDependencyAddress{Provider: parts[3], Account: parts[4], Slug: strings.TrimSuffix(parts[5], ".dep")}, nil
	}
	return contactDependencyAddress{}, fmt.Errorf("unsupported source dependency path %q", sourcePath)
}

func readContactDependencyIndex(path string) (map[string]contactDependencyEntry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return readLegacyContactDependencyIndex(strings.TrimSuffix(path, ".json") + ".jsonl")
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var index map[string]contactDependencyEntry
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return index, nil
}

func readLegacyContactDependencyIndex(path string) (map[string]contactDependencyEntry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	index := map[string]contactDependencyEntry{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry contactDependencyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		index[entry.Path] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

func writeContactDependencyIndex(path string, index map[string]contactDependencyEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".source-deps-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	data, err := json.Marshal(index)
	if err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func lockFile(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
