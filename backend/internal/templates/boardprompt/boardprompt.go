// Package boardprompt renders the board app contract appended to a loop run
// prompt when the loop is assigned to a board.
package boardprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed boardprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("boardprompt").Parse(promptTemplate))

type Data struct {
	FilePath string
	// Published is false for an app that has never shipped a version.
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
