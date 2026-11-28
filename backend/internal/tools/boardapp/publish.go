package boardapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/boards"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
)

type PublishTool struct {
	Publisher boards.MCPPublisher
}

func (t PublishTool) Definition() tools.Definition {
	return tools.Function(
		boards.PublishMCPToolName,
		boards.PublishDescription,
		false,
		boards.PublishInputSchema(),
	)
}

func (t PublishTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Publisher == nil {
		return tools.Result{}, fmt.Errorf("board app publisher is not configured")
	}
	sessionID := strings.TrimSpace(sessioncontext.SessionID(ctx))
	if sessionID == "" {
		return tools.Result{}, fmt.Errorf("%s requires a session", boards.PublishMCPToolName)
	}
	published, err := t.Publisher.PublishForSession(ctx, sessionID, boards.PublishInput{
		Title:    tools.StringInput(inputs, "title"),
		SizeHint: tools.StringInput(inputs, "size_hint"),
		HTML:     tools.StringInput(inputs, "html"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(boards.PublishOutput(published))
}
