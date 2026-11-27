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

const ShowCanvasDescription = "Presents this thread's existing canvas as a side-panel tab, or its existing main board in a canvas thread. First request the canvas module from artifact_guide, then write the canvas file. Does not create a canvas or change the thread layout."

func (t *Service) Guide(sessionID string, input GuideInput) (string, error) {
	if !slices.Contains(input.Modules, "canvas") {
		return BuildGuide(input.Modules, input.Platform), nil
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
	guide = "An editable canvas is optional in any thread. Create or edit the file below when it suits the task, then call show_canvas to present it. In an ordinary thread it opens as a side-panel tab; a canvas thread uses its existing board. Its presentation row lets the user reopen it.\n\n" + guide
	other := slices.DeleteFunc(slices.Clone(input.Modules), func(module string) bool { return module == "canvas" })
	if len(other) > 0 {
		guide += "\n\n" + BuildGuide(other, input.Platform)
	}
	return guide, nil
}

func (t *Service) AddCanvasTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowCanvasMCPToolName,
		Title:       "Show thread canvas",
		Description: ShowCanvasDescription,
	}, t.ShowCanvas)
}

func (t *Service) ShowCanvas(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	text, err := t.PresentCanvas(mcpsession.SessionID(req))
	if err != nil {
		return nil, nil, err
	}
	return textResult(text...)
}

func (t *Service) PresentCanvas(sessionID string) ([]string, error) {
	if sessionID == "" {
		return nil, errors.New("canvas presentation requires a session")
	}
	if err := t.Canvases.ValidateFile(sessionID); err != nil {
		return nil, err
	}
	return t.Present(sessionID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationCanvas})
}
