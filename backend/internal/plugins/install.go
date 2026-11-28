package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/storage"
)

type preparedPlugin struct {
	plugin   Plugin
	path     string
	source   *Source
	expected string
}

func (s *Store) publish(ctx context.Context, prepared []preparedPlugin) (out []Plugin, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, _ := s.workspaceDirs()
	ws := s.ActiveWorkspaceID()
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	existingStates := s.loadStates(ctx)
	for _, candidate := range prepared {
		if candidate.path != "" && s.reservedIDs != nil && s.reservedIDs(candidate.plugin.ID) {
			return nil, fmt.Errorf("%w: %s is a reserved built-in integration", ErrConflict, candidate.plugin.ID)
		}
		if candidate.expected != "" {
			prev := existingStates[candidate.plugin.ID]
			if prev.CommitSHA != candidate.expected {
				return nil, fmt.Errorf("%w: plugin changed while its update was fetched; retry", ErrConflict)
			}
		}
		if candidate.path != "" && candidate.source != nil {
			prev := existingStates[candidate.plugin.ID]
			if prev.SourceRepository != "" && prev.SourceRepository != candidate.source.Repository {
				return nil, fmt.Errorf("%w: plugin %q is already installed from %s; remove it first", ErrConflict, candidate.plugin.ID, prev.SourceRepository)
			}
		}
	}
	backups, err := os.MkdirTemp(dir, ".previous-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(backups)
	changed := 0
	defer func() {
		if err == nil {
			return
		}
		for i := changed - 1; i >= 0; i-- {
			live := filepath.Join(dir, prepared[i].plugin.ID)
			previous := filepath.Join(backups, prepared[i].plugin.ID)
			err = errors.Join(err, os.RemoveAll(live))
			if _, statErr := os.Lstat(previous); statErr == nil {
				err = errors.Join(err, os.Rename(previous, live))
			}
		}
	}()
	for i, candidate := range prepared {
		live := filepath.Join(dir, candidate.plugin.ID)
		previous := filepath.Join(backups, candidate.plugin.ID)
		if candidate.path == "" {
			if _, statErr := os.Stat(live); errors.Is(statErr, fs.ErrNotExist) {
				return nil, fmt.Errorf("%w: %s", ErrNotFound, candidate.plugin.ID)
			}
		}
		if err = os.Rename(live, previous); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		changed = i + 1
		if candidate.path != "" {
			if err = os.Rename(candidate.path, live); err != nil {
				return nil, err
			}
		}
	}
	out = make([]Plugin, 0, len(prepared))
	for _, candidate := range prepared {
		if candidate.path != "" {
			if s.stateStore != nil {
				prev := existingStates[candidate.plugin.ID]
				enabled := true
				if prev.PluginID != "" {
					enabled = prev.Enabled
				}
				state := storage.PluginStateRecord{
					WorkspaceID: ws,
					PluginID:    candidate.plugin.ID,
					Enabled:     enabled,
					Config:      prev.Config,
				}
				if candidate.source != nil {
					state.SourceID = candidate.source.ID
					state.SourceRepository = candidate.source.Repository
					state.SourceRef = candidate.source.Ref
					state.SourcePath = candidate.source.Path
					state.CommitSHA = candidate.source.Commit
				}
				if err = s.stateStore.SavePluginState(ctx, state); err != nil {
					return nil, err
				}
				candidate.plugin = applyState(candidate.plugin, state)
			} else {
				candidate.plugin.Source = candidate.source
			}
			out = append(out, candidate.plugin)
		} else if s.stateStore != nil {
			if err = s.stateStore.DeletePluginState(ctx, ws, candidate.plugin.ID); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s *Store) discover(repoRoot, root string) ([]preparedPlugin, error) {
	_, dataDir := s.workspaceDirs()
	if _, err := manifest.Read(root); err == nil {
		plugin, readErr := readPackage(root, dataDir, nil)
		if readErr != nil {
			return nil, readErr
		}
		return []preparedPlugin{{plugin: plugin, path: root}}, nil
	} else if !errors.Is(err, manifest.ErrNotPlugin) {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	var prepared []preparedPlugin
	seen := map[string]int{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules") {
			return fs.SkipDir
		}
		definition, err := manifest.Read(path)
		if errors.Is(err, manifest.ErrNotPlugin) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		plugin, err := readPackage(path, dataDir, nil)
		if err != nil {
			return err
		}
		if idx, exists := seen[definition.Name]; exists {
			prepared[idx].plugin = mergePluginMetadata(prepared[idx].plugin, plugin)
			return fs.SkipDir
		}
		seen[definition.Name] = len(prepared)
		prepared = append(prepared, preparedPlugin{plugin: plugin, path: path})
		return fs.SkipDir
	})
	if err == nil && len(prepared) == 0 {
		if plugin, bareErr := readPackage(root, dataDir, nil); bareErr == nil {
			return []preparedPlugin{{plugin: plugin, path: root}}, nil
		}
		err = fmt.Errorf("%w: %w", ErrInvalid, manifest.ErrNotPlugin)
	}
	_ = repoRoot
	return prepared, err
}

func stagePackages(repoRoot string, prepared []preparedPlugin, stagingRoot string, dataDir string) ([]preparedPlugin, error) {
	seen := map[string]bool{}
	for i := range prepared {
		candidate := &prepared[i]
		if seen[candidate.plugin.ID] {
			return nil, fmt.Errorf("%w: duplicate plugin %s", ErrInvalid, candidate.plugin.ID)
		}
		seen[candidate.plugin.ID] = true
		staged := filepath.Join(stagingRoot, candidate.plugin.ID)
		if err := materializeTree(repoRoot, candidate.path, staged); err != nil {
			return nil, fmt.Errorf("%w: stage %s: %v", ErrInvalid, candidate.plugin.ID, err)
		}
		materialized, err := readPackage(staged, dataDir, nil)
		if err != nil {
			return nil, err
		}
		candidate.plugin = materialized
		candidate.path = staged
	}
	return prepared, nil
}

// materializeTree copies srcDir into dstDir while dereferencing symlinks that
// resolve strictly inside repoRoot. Symlinks escaping repoRoot or looping fail.
func materializeTree(repoRoot, srcDir, dstDir string) error {
	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return err
	}
	return copyContained(resolvedRepo, srcDir, dstDir, map[string]bool{})
}

func copyContained(resolvedRepo, src, dst string, visiting map[string]bool) error {
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRepo, resolved)
	if err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return fmt.Errorf("path %q escapes repository root", src)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if visiting[resolved] {
			return fmt.Errorf("symlink cycle at %q", src)
		}
		visiting[resolved] = true
		defer delete(visiting, resolved)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(resolved)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name() == ".git" {
				continue
			}
			if err := copyContained(resolvedRepo, filepath.Join(resolved, entry.Name()), filepath.Join(dst, entry.Name()), visiting); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(resolved)
	if err != nil {
		return err
	}
	defer in.Close()
	perm := info.Mode().Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

func mergePluginMetadata(base, extra Plugin) Plugin {
	if base.DisplayName == "" || base.DisplayName == base.ID {
		if extra.DisplayName != "" {
			base.DisplayName = extra.DisplayName
		}
	}
	if base.Description == "" {
		base.Description = extra.Description
	}
	if base.Version == "" {
		base.Version = extra.Version
	}
	if base.Category == "" {
		base.Category = extra.Category
	}
	if base.Author == "" {
		base.Author = extra.Author
		base.AuthorURL = extra.AuthorURL
	}
	if base.Homepage == "" {
		base.Homepage = extra.Homepage
	}
	if base.Repository == "" {
		base.Repository = extra.Repository
	}
	if base.IconDataURL == "" && extra.IconDataURL != "" {
		base.IconDataURL = extra.IconDataURL
		base.Icon = extra.Icon
	}
	if len(base.SuggestedPrompts) == 0 && len(extra.SuggestedPrompts) > 0 {
		base.SuggestedPrompts = extra.SuggestedPrompts
	}
	if len(base.Keywords) == 0 && len(extra.Keywords) > 0 {
		base.Keywords = extra.Keywords
	}
	return base
}
