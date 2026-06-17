package memorydreamprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed memorydreamprompt.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("memorydreamprompt").Parse(promptTemplate))

type Data struct {
	Root            string
	RunSlug         string
	ReviewSlug      string
	LongTermPolicy  string
	ShortTermPolicy string
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
