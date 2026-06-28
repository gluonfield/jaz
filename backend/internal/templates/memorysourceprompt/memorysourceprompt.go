package memorysourceprompt

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed memorysourceprompt.tmpl
var promptTemplate string

//go:embed system.md
var systemPrompt string

var tmpl = template.Must(template.New("memorysourceprompt").Parse(promptTemplate))

// System returns the worker system prompt: the durable memory conventions and
// promotion bar, injected once per session so the agent never has to read the
// skill or explore the filesystem to learn them.
func System() string {
	return systemPrompt
}

type Source struct {
	Path      string
	Truncated bool
	Content   string
}

type Data struct {
	Sources []Source
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
