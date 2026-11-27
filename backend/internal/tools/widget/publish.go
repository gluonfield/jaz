package widget

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/internal/widgets"
)

type PublishTool struct {
	Publisher widgets.MCPPublisher
}

func (t PublishTool) Definition() tools.Definition {
	return tools.Function(
		widgets.PublishMCPToolName,
		widgets.PublishDescription,
		false,
		widgets.PublishInputSchema(),
	)
}

func (t PublishTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Publisher == nil {
		return tools.Result{}, fmt.Errorf("board app publisher is not configured")
	}
	sessionID := strings.TrimSpace(sessioncontext.SessionID(ctx))
	if sessionID == "" {
		return tools.Result{}, fmt.Errorf("%s requires a session", widgets.PublishMCPToolName)
	}
	published, err := t.Publisher.PublishForSession(ctx, sessionID, widgets.PublishInput{
		Title:    tools.StringInput(inputs, "title"),
		SizeHint: tools.StringInput(inputs, "size_hint"),
		HTML:     tools.StringInput(inputs, "html"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(widgets.PublishOutput(published))
}
