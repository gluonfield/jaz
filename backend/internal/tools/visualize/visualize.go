package visualize

import (
	"context"
	"errors"
	"slices"

	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	visualizesvc "github.com/wins/jaz/backend/internal/visualize"
)

type ReadMeTool struct{ Canvases *canvas.Service }

type ShowWidgetTool struct{}

func (ReadMeTool) Definition() tools.Definition {
	return tools.Function(
		visualizesvc.ReadMeMCPToolName,
		visualizesvc.ReadMeDescription,
		false,
		visualizesvc.ReadMeInputSchema(),
	)
}

func (t ReadMeTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	modules := moduleNames(inputs["modules"])
	if slices.Contains(modules, "canvas") {
		sessionID := sessioncontext.SessionID(ctx)
		if sessionID == "" {
			return tools.Result{}, errors.New("canvas guidance requires a session")
		}
		guide, err := visualizesvc.CanvasGuide(t.Canvases.Dir(sessionID))
		return tools.Result{Content: guide}, err
	}
	return tools.Result{Content: visualizesvc.BuildReadMeGuide(modules, tools.StringInput(inputs, "platform"))}, nil
}

// moduleNames leniently coerces the optional "modules" argument into a slice;
// a missing or malformed value yields the core guide rather than an error.
func moduleNames(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func (ShowWidgetTool) Definition() tools.Definition {
	return tools.Function(
		visualizesvc.ShowWidgetMCPToolName,
		"Renders a finished inline SVG, HTML fragment, or bundled HTML document in the Jaz transcript. Call the artifact guidance tool first in the turn; do not use for placeholder or plumbing-demo widgets.",
		false,
		visualizesvc.ShowWidgetInputSchema(),
	)
}

func (ShowWidgetTool) Execute(_ context.Context, inputs map[string]any) (tools.Result, error) {
	messages, err := stringSlice(inputs["loading_messages"])
	if err != nil {
		return tools.Result{}, err
	}
	output, _, err := visualizesvc.BuildArtifact(visualizesvc.ShowWidgetInput{
		LoadingMessages: messages,
		Title:           tools.StringInput(inputs, "title"),
		WidgetCode:      tools.StringInput(inputs, "widget_code"),
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Content: visualizesvc.PreparedMessage + "\n\n" + visualizesvc.DisplayReminder,
		Metadata: map[string]any{
			"status":        output.Status,
			"title":         output.Title,
			"artifact_type": output.ArtifactType,
		},
	}, nil
}

func stringSlice(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("loading_messages is required")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("loading_messages must contain strings")
		}
		out = append(out, text)
	}
	return out, nil
}
