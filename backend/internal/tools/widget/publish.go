package widget

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/internal/widgets"
)

const ToolName = "publish_widget"

type Publisher interface {
	PublishForSession(sessionID string, input widgets.PublishInput) (widgets.Widget, []string, error)
}

type Tool struct {
	Publisher Publisher
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		ToolName,
		"Publishes this loop's board widget. Use the jaztools MCP artifact guidance tool, visualize:read_me or its agent-side mapped equivalent, before building or materially changing the tile. Write the widget HTML fragment to the loop's widget file first, then call this; alternatively pass the fragment inline via html. Validation errors are returned so you can fix and retry within the run.",
		false,
		tools.ObjectSchema(map[string]any{
			"title":     tools.StringSchema("Widget title shown on the tile header. Defaults to the loop name."),
			"size_hint": tools.StringSchema("Proposed tile size as WxH (W 1-6, H 1-8), e.g. 2x2. The user can resize afterwards."),
			"html":      tools.StringSchema("Widget HTML fragment. When omitted, the loop's widget file is read instead."),
		}, []string{}),
	)
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Publisher == nil {
		return tools.Result{}, errors.New("publish_widget publisher is nil")
	}
	sessionID := sessioncontext.SessionID(ctx)
	if strings.TrimSpace(sessionID) == "" {
		return tools.Result{}, errors.New("publish_widget requires a session context")
	}
	widget, warnings, err := t.Publisher.PublishForSession(sessionID, widgets.PublishInput{
		Title:    tools.StringInput(inputs, "title"),
		SizeHint: tools.StringInput(inputs, "size_hint"),
		HTML:     tools.StringInput(inputs, "html"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	content := fmt.Sprintf("Published widget %q version %d", widget.Title, widget.CurrentVersion)
	if widget.SizeHint != "" {
		content += fmt.Sprintf(" (size %s)", widget.SizeHint)
	}
	content += "."
	if len(warnings) > 0 {
		content += "\n\nLint warnings (published anyway — fix them and republish in this run):\n- " + strings.Join(warnings, "\n- ")
	}
	return tools.Result{Content: content}, nil
}
