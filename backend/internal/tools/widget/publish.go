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
		"Publishes this loop run's Jaz board widget. Write the HTML fragment to widget/index.html first, then call this; alternatively pass the fragment inline via html.",
		false,
		widgets.PublishInputSchema(),
	)
}

func (t PublishTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Publisher == nil {
		return tools.Result{}, fmt.Errorf("widget publisher is not configured")
	}
	sessionID := strings.TrimSpace(sessioncontext.SessionID(ctx))
	if sessionID == "" {
		return tools.Result{}, fmt.Errorf("%s requires a session", widgets.PublishMCPToolName)
	}
	widget, warnings, err := t.Publisher.PublishForSession(sessionID, widgets.PublishInput{
		Title:    tools.StringInput(inputs, "title"),
		SizeHint: tools.StringInput(inputs, "size_hint"),
		HTML:     tools.StringInput(inputs, "html"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(widgets.MCPPublishOutput{
		WidgetID: widget.ID,
		Title:    widget.Title,
		Version:  widget.CurrentVersion,
		SizeHint: widget.SizeHint,
		Warnings: warnings,
	})
}
