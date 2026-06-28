package sourcequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	stateVersion     = 4
	defaultStaleTime = 30 * time.Minute
)

type Source struct {
	Path      string
	DirtyAt   time.Time
	Provider  string
	Kind      string
	MediaType string
	Key       integrations.SourceKey
	Replay    integrations.Replay
}

type Stats struct {
	Dirty      int
	Processing int
}

type Queue struct {
	Root       string
	StateFile  string
	Now        func() time.Time
	StaleAfter time.Duration

	mu sync.Mutex
}

type queueState struct {
	Version    int                         `json:"version"`
	Dirty      map[string]queuedSource     `json:"dirty,omitempty"`
	Processing map[string]processingSource `json:"processing,omitempty"`
}

type queuedSource struct {
	DirtyAt   time.Time              `json:"dirty_at"`
	Provider  string                 `json:"provider,omitempty"`
	Kind      string                 `json:"kind,omitempty"`
	MediaType string                 `json:"media_type,omitempty"`
	Key       integrations.SourceKey `json:"key,omitempty"`
	Replay    integrations.Replay    `json:"replay,omitempty"`
}

type processingSource struct {
	DirtyAt    time.Time              `json:"dirty_at"`
	ReservedAt time.Time              `json:"reserved_at"`
	Provider   string                 `json:"provider,omitempty"`
	Kind       string                 `json:"kind,omitempty"`
	MediaType  string                 `json:"media_type,omitempty"`
	Key        integrations.SourceKey `json:"key,omitempty"`
	Replay     integrations.Replay    `json:"replay,omitempty"`
}

func (s queuedSource) MarshalJSON() ([]byte, error) {
	var out struct {
		DirtyAt   time.Time               `json:"dirty_at"`
		Provider  string                  `json:"provider,omitempty"`
		Kind      string                  `json:"kind,omitempty"`
		MediaType string                  `json:"media_type,omitempty"`
		Key       *integrations.SourceKey `json:"key,omitempty"`
		Replay    *integrations.Replay    `json:"replay,omitempty"`
	}
	out.DirtyAt = s.DirtyAt
	out.Provider = s.Provider
	out.Kind = s.Kind
	out.MediaType = s.MediaType
	out.Key = sourceKeyPtr(s.Key)
	out.Replay = replayPtr(s.Replay)
	return json.Marshal(out)
}

func (s *queuedSource) UnmarshalJSON(data []byte) error {
	var raw struct {
		DirtyAt      time.Time              `json:"dirty_at"`
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
	s.DirtyAt = raw.DirtyAt
	s.Provider = raw.Provider
	s.Kind = raw.Kind
	s.MediaType = raw.MediaType
	s.Key = keyWithLegacy(raw.Key, raw.LegacyEntity, raw.LegacyDay)
	s.Replay = raw.Replay
	return nil
}

func (s processingSource) MarshalJSON() ([]byte, error) {
	var out struct {
		DirtyAt    time.Time               `json:"dirty_at"`
		ReservedAt time.Time               `json:"reserved_at"`
		Provider   string                  `json:"provider,omitempty"`
		Kind       string                  `json:"kind,omitempty"`
		MediaType  string                  `json:"media_type,omitempty"`
		Key        *integrations.SourceKey `json:"key,omitempty"`
		Replay     *integrations.Replay    `json:"replay,omitempty"`
	}
	out.DirtyAt = s.DirtyAt
	out.ReservedAt = s.ReservedAt
	out.Provider = s.Provider
	out.Kind = s.Kind
	out.MediaType = s.MediaType
	out.Key = sourceKeyPtr(s.Key)
	out.Replay = replayPtr(s.Replay)
	return json.Marshal(out)
}

func (s *processingSource) UnmarshalJSON(data []byte) error {
	var raw struct {
		DirtyAt      time.Time              `json:"dirty_at"`
		ReservedAt   time.Time              `json:"reserved_at"`
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
	s.DirtyAt = raw.DirtyAt
	s.ReservedAt = raw.ReservedAt
	s.Provider = raw.Provider
	s.Kind = raw.Kind
	s.MediaType = raw.MediaType
	s.Key = keyWithLegacy(raw.Key, raw.LegacyEntity, raw.LegacyDay)
	s.Replay = raw.Replay
	return nil
}

func keyWithLegacy(key integrations.SourceKey, entity, day string) integrations.SourceKey {
	if key != (integrations.SourceKey{}) {
		return key
	}
	return integrations.SourceKey{Entity: entity, Day: day}
}

func sourceKeyPtr(key integrations.SourceKey) *integrations.SourceKey {
	if key.IsZero() {
		return nil
	}
	return &key
}

func replayPtr(replay integrations.Replay) *integrations.Replay {
	if replay.IsZero() {
		return nil
	}
	return &replay
}

type legacyQueueState struct {
	Version    int                              `json:"version"`
	Dirty      map[string]time.Time             `json:"dirty,omitempty"`
	Processing map[string]legacyProcessingState `json:"processing,omitempty"`
}

type legacyProcessingState struct {
	DirtyAt    time.Time `json:"dirty_at"`
	ReservedAt time.Time `json:"reserved_at"`
}

func New(root string) *Queue {
	return &Queue{Root: root}
}

func (q *Queue) MarkDirtySource(ctx context.Context, source Source) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := cleanPath(source.Path)
	if err != nil {
		return err
	}
	dirtyAt := source.DirtyAt.UTC()
	if dirtyAt.IsZero() {
		dirtyAt = q.now()
	}
	source.DirtyAt = dirtyAt
	item := queuedFromSource(source)
	return q.update(func(state *queueState) error {
		if state.Dirty == nil {
			state.Dirty = map[string]queuedSource{}
		}
		if current, ok := state.Dirty[path]; !ok || shouldReplace(current, item) {
			state.Dirty[path] = item
		}
		return nil
	})
}

func (q *Queue) Reserve(ctx context.Context, limit int) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	var out []Source
	now := q.now()
	err := q.update(func(state *queueState) error {
		q.recoverStale(state, now)
		paths := make([]string, 0, len(state.Dirty))
		for path := range state.Dirty {
			paths = append(paths, path)
		}
		sort.Slice(paths, func(i, j int) bool {
			left := state.Dirty[paths[i]].DirtyAt
			right := state.Dirty[paths[j]].DirtyAt
			if left.Equal(right) {
				return paths[i] < paths[j]
			}
			return left.Before(right)
		})
		if state.Processing == nil {
			state.Processing = map[string]processingSource{}
		}
		for _, path := range paths {
			if len(out) >= limit {
				break
			}
			item := state.Dirty[path]
			dirtyAt := item.DirtyAt.UTC()
			delete(state.Dirty, path)
			state.Processing[path] = processingSource{
				DirtyAt:    dirtyAt,
				ReservedAt: now,
				Provider:   item.Provider,
				Kind:       item.Kind,
				MediaType:  item.MediaType,
				Key:        item.Key,
				Replay:     item.Replay,
			}
			out = append(out, Source{Path: path, DirtyAt: dirtyAt, Provider: item.Provider, Kind: item.Kind, MediaType: item.MediaType, Key: item.Key, Replay: item.Replay})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	var stats Stats
	err := q.view(func(state queueState) error {
		stats = Stats{Dirty: len(state.Dirty), Processing: len(state.Processing)}
		return nil
	})
	return stats, err
}

func (q *Queue) Complete(ctx context.Context, sources []Source) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return q.update(func(state *queueState) error {
		return completeSources(state, sources)
	})
}

func (q *Queue) Release(ctx context.Context, sources []Source) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return q.update(func(state *queueState) error {
		return releaseSources(state, sources)
	})
}

func (q *Queue) Settle(ctx context.Context, completed, failed []Source) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return q.update(func(state *queueState) error {
		if err := completeSources(state, completed); err != nil {
			return err
		}
		return releaseSources(state, failed)
	})
}

func completeSources(state *queueState, sources []Source) error {
	for _, source := range sources {
		path, err := cleanPath(source.Path)
		if err != nil {
			return err
		}
		processing, ok := state.Processing[path]
		if ok && processing.DirtyAt.Equal(source.DirtyAt.UTC()) {
			delete(state.Processing, path)
		}
	}
	return nil
}

func releaseSources(state *queueState, sources []Source) error {
	if state.Dirty == nil {
		state.Dirty = map[string]queuedSource{}
	}
	for _, source := range sources {
		path, err := cleanPath(source.Path)
		if err != nil {
			return err
		}
		processing, ok := state.Processing[path]
		item := queuedFromSource(source)
		if ok {
			item = queuedFromProcessing(processing)
			delete(state.Processing, path)
		}
		if current, ok := state.Dirty[path]; !ok || shouldReplace(current, item) {
			state.Dirty[path] = item
		}
	}
	return nil
}

func (q *Queue) update(fn func(*queueState) error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	unlock, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock()
	state, err := q.load()
	if err != nil {
		return err
	}
	if err := fn(&state); err != nil {
		return err
	}
	return q.save(state)
}

func (q *Queue) view(fn func(queueState) error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	unlock, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock()
	state, err := q.load()
	if err != nil {
		return err
	}
	return fn(state)
}

func (q *Queue) load() (queueState, error) {
	path, err := q.statePath()
	if err != nil {
		return queueState{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newState(), nil
	}
	if err != nil {
		return queueState{}, err
	}
	if len(data) == 0 {
		return newState(), nil
	}
	state, err := parseState(data)
	if err != nil {
		return queueState{}, err
	}
	return state, nil
}

func parseState(data []byte) (queueState, error) {
	var state queueState
	if err := json.Unmarshal(data, &state); err != nil {
		legacy, legacyErr := parseLegacyState(data)
		if legacyErr == nil {
			return legacy, nil
		}
		return queueState{}, err
	}
	if state.Version == 0 {
		state.Version = stateVersion
	}
	return state, nil
}

func parseLegacyState(data []byte) (queueState, error) {
	var legacy legacyQueueState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return queueState{}, err
	}
	state := newState()
	if len(legacy.Dirty) > 0 {
		state.Dirty = map[string]queuedSource{}
		for path, dirtyAt := range legacy.Dirty {
			clean, err := cleanPath(path)
			if err != nil {
				return queueState{}, err
			}
			state.Dirty[clean] = queuedSource{DirtyAt: dirtyAt.UTC()}
		}
	}
	if len(legacy.Processing) > 0 {
		state.Processing = map[string]processingSource{}
		for path, processing := range legacy.Processing {
			clean, err := cleanPath(path)
			if err != nil {
				return queueState{}, err
			}
			state.Processing[clean] = processingSource{
				DirtyAt:    processing.DirtyAt.UTC(),
				ReservedAt: processing.ReservedAt.UTC(),
			}
		}
	}
	return state, nil
}

func (q *Queue) save(state queueState) error {
	state.Version = stateVersion
	if len(state.Dirty) == 0 {
		state.Dirty = nil
	}
	if len(state.Processing) == 0 {
		state.Processing = nil
	}
	path, err := q.statePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".dirty-sources-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	data, err := json.Marshal(state)
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

func (q *Queue) recoverStale(state *queueState, now time.Time) {
	staleAfter := q.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultStaleTime
	}
	if state.Dirty == nil {
		state.Dirty = map[string]queuedSource{}
	}
	for path, processing := range state.Processing {
		if now.Sub(processing.ReservedAt) < staleAfter {
			continue
		}
		item := queuedFromProcessing(processing)
		if current, ok := state.Dirty[path]; !ok || shouldReplace(current, item) {
			state.Dirty[path] = item
		}
		delete(state.Processing, path)
	}
}

func queuedFromSource(source Source) queuedSource {
	return queuedSource{
		DirtyAt:   source.DirtyAt.UTC(),
		Provider:  source.Provider,
		Kind:      source.Kind,
		MediaType: source.MediaType,
		Key:       source.Key,
		Replay:    source.Replay,
	}
}

func queuedFromProcessing(processing processingSource) queuedSource {
	return queuedSource{
		DirtyAt:   processing.DirtyAt,
		Provider:  processing.Provider,
		Kind:      processing.Kind,
		MediaType: processing.MediaType,
		Key:       processing.Key,
		Replay:    processing.Replay,
	}
}

func shouldReplace(current, next queuedSource) bool {
	if current.DirtyAt.Before(next.DirtyAt) {
		return true
	}
	if !current.DirtyAt.Equal(next.DirtyAt) {
		return false
	}
	if current.Kind == "" && next.Kind != "" {
		return true
	}
	if current.Key == (integrations.SourceKey{}) && next.Key != (integrations.SourceKey{}) {
		return true
	}
	return len(current.Replay.Scopes) == 0 && len(next.Replay.Scopes) > 0
}

func (q *Queue) statePath() (string, error) {
	root := strings.TrimSpace(q.Root)
	if root == "" {
		return "", fmt.Errorf("queue root is required")
	}
	stateFile := strings.TrimSpace(q.StateFile)
	if stateFile == "" {
		stateFile = filepath.Join(".state", "dirty-sources.json")
	}
	clean := filepath.Clean(filepath.FromSlash(stateFile))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("queue state path escapes root")
	}
	return filepath.Join(root, clean), nil
}

func (q *Queue) lock() (func(), error) {
	path, err := q.statePath()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
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

func (q *Queue) now() time.Time {
	now := time.Now().UTC()
	if q.Now != nil {
		now = q.Now().UTC()
	}
	return now
}

func newState() queueState {
	return queueState{Version: stateVersion}
}

func cleanPath(value string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source path escapes memory root")
	}
	return filepath.ToSlash(clean), nil
}
