package visualize

import (
	"context"
	"errors"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/templates/jazplatform"
)

const ShowCanvasDescription = "Presents this thread's existing canvas as a side-panel tab, or its existing main board in a canvas thread. First request the canvas module from visualise_read_me, then write the canvas file. Does not create a canvas or change the thread layout."

func (t *MCPTools) Guide(sessionID string, input ReadMeInput) (string, error) {
	if !slices.Contains(input.Modules, "canvas") {
		return BuildReadMeGuide(input.Modules, input.Platform), nil
	}
	if sessionID == "" {
		return "", errors.New("canvas guidance requires a session")
	}
	if _, err := t.Sessions.LoadSession(sessionID); err != nil {
		return "", err
	}
	guide, err := jazplatform.RenderCanvas(t.Canvases.Dir(sessionID))
	if err != nil {
		return "", err
	}
	guide = "An editable canvas is optional in any thread. Create or edit the file below when it suits the task, then call visualise_show_canvas to present it. In an ordinary thread it opens as a side-panel tab; a canvas thread uses its existing board. Its presentation row lets the user reopen it.\n\n" + guide
	other := slices.DeleteFunc(slices.Clone(input.Modules), func(module string) bool { return module == "canvas" })
	if len(other) > 0 {
		guide += "\n\n" + BuildReadMeGuide(other, input.Platform)
	}
	return guide, nil
}

func (t *MCPTools) AddCanvasTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowCanvasMCPToolName,
		Title:       "Show thread canvas",
		Description: ShowCanvasDescription,
	}, t.ShowCanvas)
}

func (t *MCPTools) ShowCanvas(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	return t.PresentCanvas(mcpsession.SessionID(req))
}

func (t *MCPTools) PresentCanvas(sessionID string) (*mcp.CallToolResult, any, error) {
	if sessionID == "" {
		return nil, nil, errors.New("canvas presentation requires a session")
	}
	if err := t.Canvases.ValidateFile(sessionID); err != nil {
		return nil, nil, err
	}
	return t.Present(sessionID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationCanvas})
}
