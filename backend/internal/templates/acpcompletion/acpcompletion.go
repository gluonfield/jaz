// Package acpcompletion renders the synthetic turn that asks the native
// agent to report a finished ACP session's outcome to the user.
package acpcompletion

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed acpcompletion.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("acpcompletion").Parse(promptTemplate))

type Data struct {
	Slug      string
	Agent     string
	State     string
	Error     string
	Assistant string
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
