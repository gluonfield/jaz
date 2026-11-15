package visualize

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/templates/jazplatform"
)

func (t *MCPTools) canvasGuide(req *mcp.CallToolRequest) (*mcp.CallToolResult, any, error) {
	sessionID := mcpsession.SessionID(req)
	if _, err := t.Sessions.LoadSession(sessionID); err != nil {
		return nil, nil, err
	}
	guide, err := CanvasGuide(t.Canvases.Dir(sessionID))
	if err != nil {
		return nil, nil, err
	}
	return textResult(guide)
}

func CanvasGuide(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("canvas guidance requires a session")
	}
	guide, err := jazplatform.RenderCanvas(dir)
	return "An editable canvas is optional in any thread. Create or edit the file below when it suits the task, then call visualise_show_canvas to present it. In an ordinary thread it opens as a side-panel tab; a canvas thread uses its existing board. A Markdown link to the file lets the user reopen it.\n\n" + guide, err
}

func (t *MCPTools) AddCanvasTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowCanvasMCPToolName,
		Title:       "Show thread canvas",
		Description: "Presents this thread's existing canvas as a side-panel tab, or its existing main board in a canvas thread. First request the canvas module from visualise_read_me, then write the canvas file. Does not create a canvas or change the thread layout.",
	}, t.ShowCanvas)
}

func (t *MCPTools) ShowCanvas(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	sessionID := mcpsession.SessionID(req)
	if _, err := t.Sessions.LoadSession(sessionID); err != nil {
		return nil, nil, err
	}
	file, err := t.Canvases.ExistingFile(sessionID)
	if err != nil {
		return nil, nil, err
	}
	return t.Present(req, &sessionevents.ArtifactEvent{Title: "Canvas", ArtifactType: "canvas", File: file})
}
