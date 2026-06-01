package acp

import (
	"fmt"
	"path/filepath"
	"strings"
)

func (m *Manager) resolveCwd(configured string) (string, error) {
	cwd := firstNonEmpty(configured, m.cfg.Workspace)
	if cwd == "" {
		cwd = "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if m.cfg.Workspace == "" {
		return abs, nil
	}
	workspace, err := filepath.Abs(m.cfg.Workspace)
	if err != nil {
		return "", err
	}
	if !isWithin(workspace, abs) {
		return "", fmt.Errorf("acp cwd escapes workspace: %s", cwd)
	}
	return abs, nil
}

func safePath(root, name string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(rootAbs, name)
	if filepath.IsAbs(name) {
		abs = filepath.Clean(name)
	}
	if !isWithin(rootAbs, abs) {
		return "", fmt.Errorf("path escapes workspace: %s", name)
	}
	return abs, nil
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
