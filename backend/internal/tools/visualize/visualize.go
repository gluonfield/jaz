package visualize

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	visualizesvc "github.com/wins/jaz/backend/internal/visualize"
)

type GuideTool struct{ Visualize *visualizesvc.Service }

type ShowCanvasTool struct{ Visualize *visualizesvc.Service }

type ShowArtifactTool struct{}

func (GuideTool) Definition() tools.Definition {
	return tools.Function(
		visualizesvc.GuideMCPToolName,
		visualizesvc.GuideDescription,
		false,
		visualizesvc.GuideInputSchema(),
	)
}

func (t GuideTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	guide, err := t.Visualize.Guide(sessioncontext.SessionID(ctx), visualizesvc.GuideInput{Modules: moduleNames(inputs["modules"]), Platform: tools.StringInput(inputs, "platform")})
	return tools.Result{Content: guide}, err
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

func (ShowArtifactTool) Definition() tools.Definition {
	return tools.Function(
		visualizesvc.ShowArtifactMCPToolName,
		visualizesvc.ShowArtifactDescription,
		false,
		visualizesvc.ShowArtifactInputSchema(),
	)
}

func (ShowArtifactTool) Execute(_ context.Context, inputs map[string]any) (tools.Result, error) {
	messages, err := stringSlice(inputs["loading_messages"])
	if err != nil {
		return tools.Result{}, err
	}
	output, _, err := visualizesvc.BuildArtifact(visualizesvc.ShowArtifactInput{
		LoadingMessages: messages,
		Title:           tools.StringInput(inputs, "title"),
		HTML:            tools.StringInput(inputs, "html"),
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
