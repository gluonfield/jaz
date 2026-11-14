package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const ShowViewMCPToolName = "show_view"

// Presenter shows an artifact in the thread the call came from.
type Presenter interface {
	Present(req *mcp.CallToolRequest, artifact *sessionevents.ArtifactEvent) (*mcp.CallToolResult, any, error)
}

type MCPTools struct {
	Plugins   *Store
	Presenter Presenter
}

type ShowViewInput struct {
	View  string         `json:"view" jsonschema:"The view to show, as <plugin>/<view>, for example tenders/status."`
	Title string         `json:"title" jsonschema:"A short title for the card or the side-panel tab."`
	Data  map[string]any `json:"data,omitempty" jsonschema:"What the view draws, as a JSON object. Shows the view as a card in the conversation."`
	File  string         `json:"file,omitempty" jsonschema:"A JSON file in the thread's folder holding what the view draws. Opens the view in the side panel, where it redraws whenever the file changes. Use instead of data for a view that should stay in sight while work goes on."`
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowViewMCPToolName,
		Title:       "Show a plugin's view",
		Description: "Shows one of an installed plugin's own views, named by the skill that asks for it. Pass data for a card in the conversation, or file for a view kept open in the side panel. Do not write the view's HTML yourself.",
	}, t.ShowView)
}

func (t *MCPTools) ShowView(_ context.Context, req *mcp.CallToolRequest, input ShowViewInput) (*mcp.CallToolResult, any, error) {
	title := strings.TrimSpace(input.Title)
	file := strings.TrimSpace(input.File)
	if title == "" {
		return nil, nil, errors.New("title is required")
	}
	if (input.Data == nil) == (file == "") {
		return nil, nil, errors.New("pass either data, for a card in the conversation, or file, for a view in the side panel")
	}
	if file != "" && !filepath.IsLocal(file) {
		return nil, nil, errors.New("file must be a path inside the thread's folder")
	}
	html, err := t.Plugins.View(strings.TrimSpace(input.View))
	if err != nil {
		return nil, nil, err
	}
	artifact := &sessionevents.ArtifactEvent{Title: title, WidgetCode: html, ArtifactType: "html", File: file}
	if input.Data != nil {
		if artifact.Data, err = json.Marshal(input.Data); err != nil {
			return nil, nil, err
		}
	}
	return t.Presenter.Present(req, artifact)
}
