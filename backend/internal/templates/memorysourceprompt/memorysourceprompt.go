package memorysourceprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed memorysourceprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("memorysourceprompt").Parse(promptTemplate))

type Source struct {
	Path      string
	Truncated bool
	Content   string
}

type Data struct {
	Root    string
	Sources []Source
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
