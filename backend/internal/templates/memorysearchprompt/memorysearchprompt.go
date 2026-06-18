package memorysearchprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed memorysearchprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("memorysearchprompt").Parse(promptTemplate))

type Data struct {
	Query string
	Deep  bool
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
