// Package jazagent renders native Jaz's operating prompt: identity and
// delegation rules for the built-in coordinator. Everything shared with other
// agents (runtime paths, AGENTS.md, SOUL.md, INTERNAL.md, memory, skills) lives
// in jazplatform.
package jazagent

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed jazagent.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("jazagent").Parse(promptTemplate))

func Render() (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, nil)
	return out.String(), err
}
