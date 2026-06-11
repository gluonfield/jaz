// Package widgetprompt renders the widget contract section appended to a
// loop run prompt when the loop is assigned to a board.
package widgetprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed widgetprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("widgetprompt").Parse(promptTemplate))

type Data struct {
	FilePath  string
	GuidePath string
	// Published is false for a widget that has never shipped a version.
	Published      bool
	Version        int
	Title          string
	SizeHint       string
	LastError      string
	LayoutFeedback string
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
