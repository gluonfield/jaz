package visualize

import (
	"context"
	"strings"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	visualizesvc "github.com/wins/jaz/backend/internal/visualize"
)

func (ShowCanvasTool) Definition() tools.Definition {
	return tools.Function(visualizesvc.ShowCanvasMCPToolName, visualizesvc.ShowCanvasDescription, false, tools.ObjectSchema(map[string]any{}, []string{}))
}

func (t ShowCanvasTool) Execute(ctx context.Context, _ map[string]any) (tools.Result, error) {
	text, err := t.Visualize.PresentCanvas(sessioncontext.SessionID(ctx))
	return tools.Result{Content: strings.Join(text, "\n\n")}, err
}
