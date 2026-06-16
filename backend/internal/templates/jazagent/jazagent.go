// Package jazagent renders the native coordinator's operating prompt:
// identity, environment, and delegation rules — how the jaz agent itself
// works, analogous to Claude Code's or codex's own system prompt. Everything
// shared with other agents (AGENTS.md, SOUL.md, memory, skills) lives in
// jazplatform instead.
package jazagent

import (
	_ "embed"
)

//go:embed jazagent.tmpl
var promptTemplate string

func Render() string {
	return promptTemplate
}
