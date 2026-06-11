// Package skillsprompt renders the skills catalog block of the system prompt.
package skillsprompt

import (
	"bytes"
	_ "embed"
	"html"
	"text/template"
)

//go:embed skillsprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("skillsprompt").Funcs(template.FuncMap{
	"escape": html.EscapeString,
}).Parse(promptTemplate))

type Skill struct {
	Name        string
	Description string
	Location    string
}

type Data struct {
	Skills []Skill
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
