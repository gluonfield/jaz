package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const ShowViewMCPToolName = "show_view"

type Presenter interface {
	PresentMCP(sessionID string, artifact *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error)
}

type MCPTools struct {
	Plugins   *Store
	Presenter Presenter
}

type ShowViewInput struct {
	View  string         `json:"view" jsonschema:"The view to show, as <plugin>/<view>, for example tenders/status."`
	Title string         `json:"title" jsonschema:"A short title for the card or the side-panel tab."`
	Data  map[string]any `json:"data,omitempty" jsonschema:"What the view draws, as a JSON object. Shows the view as a card in the conversation and persists structured view state."`
	File  string         `json:"file,omitempty" jsonschema:"A clean workspace-relative JSON file path in the thread's folder (for example status.json or runs/scene.json) holding what the view draws. Opens the view in the side panel, where it redraws whenever the file changes."`
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowViewMCPToolName,
		Title:       "Show a plugin's view",
		Description: "Shows one of an installed plugin's own views, named by the skill that asks for it. Pass data for a card in the conversation, or file for a view kept open in the side panel. Do not write the view's HTML yourself.",
	}, t.ShowView)
}

func (t *MCPTools) ShowView(ctx context.Context, req *mcp.CallToolRequest, input ShowViewInput) (*mcp.CallToolResult, any, error) {
	title := strings.TrimSpace(input.Title)
	file := strings.TrimSpace(input.File)
	if title == "" {
		return nil, nil, errors.New("title is required")
	}
	if (input.Data == nil) == (file == "") {
		return nil, nil, errors.New("pass either data, for a card in the conversation, or file, for a view in the side panel")
	}
	if file != "" && !validViewFile(file) {
		return nil, nil, errors.New("file must be a clean relative path inside the thread's folder")
	}
	ref := strings.TrimSpace(input.View)
	html, err := t.Plugins.View(ref)
	if err != nil {
		return nil, nil, err
	}
	sessionID := mcpsession.SessionID(req)
	if file != "" {
		return t.Presenter.PresentMCP(sessionID, &sessionevents.PresentationEvent{
			Kind: sessionevents.PresentationPluginView,
			View: &sessionevents.PluginView{Reference: ref, Title: title, WidgetCode: html, File: file},
		})
	}
	raw, err := json.Marshal(input.Data)
	if err != nil {
		return nil, nil, err
	}
	if pluginID, viewName, ok := strings.Cut(ref, "/"); ok && sessionID != "" && t.Plugins != nil && t.Plugins.stateStore != nil {
		_ = t.Plugins.stateStore.SaveThreadPluginData(ctx, sessionID, pluginID, viewName, "", raw)
	}
	artifact := &sessionevents.ArtifactEvent{Title: title, WidgetCode: html, ArtifactType: "html", Data: raw}
	return t.Presenter.PresentMCP(sessionID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationArtifact, Artifact: artifact})
}
