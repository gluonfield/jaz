package visualize

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	visualizesvc "github.com/wins/jaz/backend/internal/visualize"
)

func (ShowCanvasTool) Definition() tools.Definition {
	return tools.Function(visualizesvc.ShowCanvasMCPToolName, visualizesvc.ShowCanvasDescription, false, tools.ObjectSchema(map[string]any{}, []string{}))
}

func (t ShowCanvasTool) Execute(ctx context.Context, _ map[string]any) (tools.Result, error) {
	result, _, err := t.Visualize.PresentCanvas(sessioncontext.SessionID(ctx))
	if err != nil {
		return tools.Result{}, err
	}
	var text []string
	for _, content := range result.Content {
		if content, ok := content.(*mcp.TextContent); ok {
			text = append(text, content.Text)
		}
	}
	return tools.Result{Content: strings.Join(text, "\n\n")}, nil
}
