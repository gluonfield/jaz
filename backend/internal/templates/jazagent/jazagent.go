// Package jazagent renders the native coordinator's operating prompt:
// identity, environment, and delegation rules — how the jaz agent itself
// works, analogous to Claude Code's or codex's own system prompt. Everything
// shared with other agents (AGENTS.md, SOUL.md, memory, skills) lives in
// jazplatform instead.
package jazagent

import (
	"bytes"
	_ "embed"
	"text/template"
	"time"
)

//go:embed jazagent.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("jazagent").Parse(promptTemplate))

type data struct {
	Date     string
	Time     string
	Timezone string
	Weekday  string
	Human    string
	Cwd      string
}

func Render(now time.Time, cwd string) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data{
		Date:     now.Format("January 2, 2006"),
		Time:     now.Format("15:04:05 MST"),
		Timezone: now.Format("MST (UTCZ07:00)"),
		Weekday:  now.Format("Monday"),
		Human:    now.Format("Monday, January 2, 2006 at 15:04:05 MST"),
		Cwd:      cwd,
	})
	return out.String(), err
}
