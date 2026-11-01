// Package jazplatform renders separate thread and bot prompt extensions.
package jazplatform

import (
	"bytes"
	"embed"
	"strings"
	"text/template"

	"github.com/wins/jaz/backend/internal/connections"
)

//go:embed *.tmpl
var promptTemplates embed.FS

var tmpl = template.Must(template.New("jazplatform").ParseFS(promptTemplates, "*.tmpl"))

// MemoryData carries the live jazmem horizons. LongTerm and ShortTerm always
// render (callers pass "(empty)" placeholders when blank); today's daily page
// renders only when it has content — SHORT_TERM.md is the curated carry-over,
// so older dailies are not injected. Nil MemoryData means memory is disabled
// and the whole memory block, protocol included, is omitted.
type MemoryData struct {
	Root                string
	LongTerm            string
	ShortTerm           string
	ShortTermCharacters int
	TodayName           string
	Today               string
}

type RuntimePaths struct {
	Root             string
	AgentsPath       string
	SoulPath         string
	SkillsPath       string
	SessionsPath     string
	DefaultWorkspace string
	WorktreesPath    string
}

type Data struct {
	Agents          string
	AgentNames      []string
	Date            string
	Time            string
	Timezone        string
	Weekday         string
	Human           string
	Cwd             string
	Device          string
	RuntimePaths    RuntimePaths
	Soul            string
	BrowserEnabled  bool
	ArtifactSurface string
	CanvasDir       string
	Memory          *MemoryData
	Connections     []connections.AgentConnection
	Skills          string
}

type BotData struct {
	Data
	Instructions string
}

func RenderThread(data Data) (string, error) {
	return renderTemplate("thread.tmpl", data.normalized())
}

func RenderBot(data BotData) (string, error) {
	data.Data = data.Data.normalized()
	return renderTemplate("bot.tmpl", data)
}

func RenderBotPolicy() (string, error) {
	return renderPromptModule("bot_policy", nil)
}

func (data Data) normalized() Data {
	data.Device = deviceLabel(data.Device)
	data.ArtifactSurface = strings.TrimSpace(data.ArtifactSurface)
	if data.ArtifactSurface != "widget" && data.ArtifactSurface != "canvas" {
		data.ArtifactSurface = "chat"
	}
	return data
}

func RenderConnections(connections []connections.AgentConnection) (string, error) {
	return renderPromptModule("connections", connections)
}

func RenderMemory(memory *MemoryData) (string, error) {
	return renderPromptModule("memory", memory)
}

func renderPromptModule(name string, data any) (string, error) {
	out, err := renderTemplate(name, data)
	return strings.TrimSpace(out), err
}

func renderTemplate(name string, data any) (string, error) {
	var out bytes.Buffer
	err := tmpl.ExecuteTemplate(&out, name, data)
	return out.String(), err
}

func deviceLabel(device string) string {
	if device == "mobile" {
		return "Mobile"
	}
	return "Desktop"
}
