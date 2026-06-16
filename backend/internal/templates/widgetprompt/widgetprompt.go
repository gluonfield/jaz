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
	FilePath string
	// FileExists is checked at render time so the agent is told whether to
	// iterate on the file or create it, instead of discovering via a failed read.
	FileExists bool
	// Published is false for a widget that has never shipped a version.
	Published      bool
	Version        int
	Title          string
	SizeHint       string
	LastError      string
	LayoutFeedback string
}

func Render(data Data) string {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic(err)
	}
	return out.String()
}
