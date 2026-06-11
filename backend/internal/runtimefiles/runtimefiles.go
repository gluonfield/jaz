package runtimefiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	Root             string
	Sessions         string
	Workspaces       string
	DefaultWorkspace string
	UserSkills       string
	ManagedSkills    string
	Automations      string
	ACPHome          string
	ACPCodexHome     string
	ACPTmp           string
	ACPNPMCache      string
}

func New(root string) Layout {
	root = strings.TrimSpace(root)
	return Layout{
		Root:             root,
		Sessions:         filepath.Join(root, "sessions"),
		Workspaces:       filepath.Join(root, "workspaces"),
		DefaultWorkspace: filepath.Join(root, "workspaces", "default"),
		UserSkills:       filepath.Join(root, "skills"),
		ManagedSkills:    filepath.Join(root, "system", "skills"),
		Automations:      filepath.Join(root, "automations"),
		ACPHome:          filepath.Join(root, "acp", "home"),
		ACPCodexHome:     filepath.Join(root, "acp", "codex-home"),
		ACPTmp:           filepath.Join(root, "acp", "tmp"),
		ACPNPMCache:      filepath.Join(root, "acp", "npm-cache"),
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
		layout.ACPHome,
		layout.ACPCodexHome,
		layout.ACPTmp,
		layout.ACPNPMCache,
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Layout{}, err
		}
	}
	return layout, nil
}
