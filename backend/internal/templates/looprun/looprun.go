package looprun

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed looprun.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("looprun").Parse(promptTemplate))

// Data renders the per-run system prompt extension for one loop run.
type Data struct {
	LoopName     string
	LoopID       string
	RunID        string
	ScheduledFor string
	Now          string
	MemoryPath   string
	PreviousRun  string
	Extras       []string
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
