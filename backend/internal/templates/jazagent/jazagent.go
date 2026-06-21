// Package jazagent renders the Jaz ACP agent's operating prompt:
// identity, environment, and delegation rules — how the jaz agent itself
// works, analogous to Claude Code's or codex's own system prompt. Everything
// shared with other agents (AGENTS.md, SOUL.md, memory, skills) lives in
// jazplatform instead.
package jazagent

import (
	"bytes"
	_ "embed"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed jazagent.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("jazagent").Parse(promptTemplate))

type Data struct {
	Root             string
	AgentsPath       string
	SoulPath         string
	SkillsPath       string
	SessionsPath     string
	DefaultWorkspace string
	WorktreesPath    string
}

func Render(root, workspace string) (string, error) {
	data := guideData(root, workspace)
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}

func guideData(root, workspace string) Data {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "~/.jaz"
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = filepath.Join(root, "workspaces", "default")
	}
	return Data{
		Root:             root,
		AgentsPath:       filepath.Join(root, "AGENTS.md"),
		SoulPath:         filepath.Join(root, "SOUL.md"),
		SkillsPath:       filepath.Join(root, "skills"),
		SessionsPath:     filepath.Join(root, "sessions"),
		DefaultWorkspace: workspace,
		WorktreesPath:    filepath.Join(workspace, ".worktrees"),
	}
}
