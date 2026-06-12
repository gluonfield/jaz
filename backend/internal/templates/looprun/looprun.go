package looprun

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed looprun.tmpl
var promptTemplate string

var tmpl = template.Must(template.New("looprun").Parse(promptTemplate))

// Data renders the prompt for one loop run: context and rules up top,
// capability sections (widget instructions, …) in the middle, and the user's
// task last so it lands with the most emphasis.
type Data struct {
	LoopName     string
	LoopID       string
	RunID        string
	ScheduledFor string
	Now          string
	MemoryPath   string
	// MemoryExists is checked at render time so the agent is told whether to
	// read the memory file or skip straight to a fresh start.
	MemoryExists bool
	PreviousRun  string
	Extras       []string
	Prompt       string
}

func Render(data Data) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, data)
	return out.String(), err
}
