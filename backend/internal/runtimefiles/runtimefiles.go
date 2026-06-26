package runtimefiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	Root              string
	Sessions          string
	Workspaces        string
	DefaultWorkspace  string
	UserSkills        string
	Automations       string
	Connections       string
	Ingest            string
	ACPCodexHome      string
	ACPClaudeConfig   string
	ACPOpenCodeConfig string
}

func New(root string) Layout {
	root = strings.TrimSpace(root)
	return Layout{
		Root:              root,
		Sessions:          filepath.Join(root, "sessions"),
		Workspaces:        filepath.Join(root, "workspaces"),
		DefaultWorkspace:  filepath.Join(root, "workspaces", "default"),
		UserSkills:        filepath.Join(root, "skills"),
		Automations:       filepath.Join(root, "automations"),
		Connections:       filepath.Join(root, "connections"),
		Ingest:            filepath.Join(root, "ingest"),
		ACPCodexHome:      filepath.Join(root, "acp", "codex-home"),
		ACPClaudeConfig:   filepath.Join(root, "acp", "claude"),
		ACPOpenCodeConfig: filepath.Join(root, "acp", "opencode"),
	}
}

func Ensure(root string) (Layout, error) {
	layout := New(root)
	if layout.Root == "" {
		return Layout{}, fmt.Errorf("runtime root is empty")
	}
	for _, dir := range []string{
		layout.Root,
		layout.Sessions,
		layout.Workspaces,
		layout.DefaultWorkspace,
		layout.UserSkills,
		layout.Automations,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Layout{}, err
		}
	}
	for _, dir := range []string{
		layout.ACPCodexHome,
		layout.ACPClaudeConfig,
		layout.ACPOpenCodeConfig,
		layout.Connections,
		layout.Ingest,
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Layout{}, err
		}
	}
	return layout, nil
}
