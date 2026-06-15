package visualize

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	ReadMeMCPToolName     = "visualize:read_me"
	ShowWidgetMCPToolName = "visualize:show_widget"
	ReadMeToolName        = "visualize_read_me"
	ShowWidgetToolName    = "visualize_show_widget"

	MaxWidgetCodeBytes = 5 << 20
)

const (
	RenderedMessage  = "Content rendered and shown to the user. Please do not duplicate the shown content in text because it's already visually represented."
	RenderedReminder = "[This tool call rendered an interactive widget in the chat. The user can already see the result — do not repeat it in text or with another visualization tool.]"
)

//go:embed read_me.md
var ReadMeGuide string

type SessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type SessionEventPublisher interface {
	Publish(event sessionevents.Event)
}

type ReadMeInput struct {
	Modules  []string `json:"modules,omitempty" jsonschema:"diagram, mockup, interactive, data_viz, art, chart, elicitation"`
	Platform string   `json:"platform,omitempty" jsonschema:"mobile, desktop, unknown"`
}

type ShowWidgetInput struct {
	LoadingMessages []string `json:"loading_messages" jsonschema:"1-4 short strings shown while the artifact renders"`
	Title           string   `json:"title" jsonschema:"short artifact title"`
	WidgetCode      string   `json:"widget_code" jsonschema:"raw SVG, HTML fragment, or full self-contained HTML document"`
}

type ShowWidgetOutput struct {
	Status       string `json:"status"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type"`
}

type MCPTools struct {
	Store  SessionEventAppender
	Events SessionEventPublisher
}

func NewMCPTools(store SessionEventAppender, events SessionEventPublisher) *MCPTools {
	return &MCPTools{Store: store, Events: events}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ReadMeMCPToolName,
		Title:       "Read artifact guidance",
		Description: "Loads design-system and module guidance before creating a real SVG or HTML inline artifact. Call this silently before the first visual artifact in a turn.",
		InputSchema: readMeInputSchema(),
	}, t.ReadMe)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowWidgetMCPToolName,
		Title:       "Show inline artifact",
		Description: "Renders a finished SVG, HTML fragment, or full self-contained HTML document inline in the Jaz transcript. Do not use for placeholder or plumbing-demo widgets.",
		InputSchema: showWidgetInputSchema(),
	}, t.ShowWidget)
}

func (t *MCPTools) ReadMe(context.Context, *mcp.CallToolRequest, ReadMeInput) (*mcp.CallToolResult, map[string]string, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ReadMeGuide}}}, map[string]string{"status": "ok"}, nil
}

func (t *MCPTools) ShowWidget(_ context.Context, req *mcp.CallToolRequest, input ShowWidgetInput) (*mcp.CallToolResult, ShowWidgetOutput, error) {
	output, artifact, err := BuildArtifact(input)
	if err != nil {
		return nil, ShowWidgetOutput{}, err
	}
	if sessionID := sessionIDFromRequest(req); sessionID != "" && t.Store != nil {
		event := sessionevents.Event{
			SessionID: sessionID,
			Type:      sessionevents.TypeArtifact,
			Artifact:  artifact,
		}
		events := []sessionevents.Event{event}
		if err := t.Store.AppendSessionEvents(sessionID, events...); err != nil {
			return nil, ShowWidgetOutput{}, err
		}
		if t.Events != nil {
			t.Events.Publish(events[0])
		}
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: RenderedMessage},
		&mcp.TextContent{Text: RenderedReminder},
	}}, output, nil
}

func BuildArtifact(input ShowWidgetInput) (ShowWidgetOutput, *sessionevents.ArtifactEvent, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return ShowWidgetOutput{}, nil, errors.New("title is required")
	}
	code := input.WidgetCode
	if strings.TrimSpace(code) == "" {
		return ShowWidgetOutput{}, nil, errors.New("widget_code is required")
	}
	codeBytes := len([]byte(code))
	if codeBytes > MaxWidgetCodeBytes {
		return ShowWidgetOutput{}, nil, fmt.Errorf("widget_code exceeds %d bytes", MaxWidgetCodeBytes)
	}
	messages, err := normalizeLoadingMessages(input.LoadingMessages)
	if err != nil {
		return ShowWidgetOutput{}, nil, err
	}
	kind := ArtifactKind(code)
	output := ShowWidgetOutput{
		Status:       "ok",
		Title:        title,
		ArtifactType: kind,
	}
	return output, &sessionevents.ArtifactEvent{
		Title:           title,
		WidgetCode:      code,
		LoadingMessages: messages,
		ArtifactType:    kind,
	}, nil
}

func ArtifactKind(code string) string {
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(code)), "<svg") {
		return "svg"
	}
	return "html"
}

func normalizeLoadingMessages(messages []string) ([]string, error) {
	out := make([]string, 0, min(len(messages), 4))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message != "" {
			out = append(out, message)
		}
		if len(out) == 4 {
			break
		}
	}
	if len(out) == 0 {
		return nil, errors.New("loading_messages must contain at least one non-empty string")
	}
	return out, nil
}

func sessionIDFromRequest(req *mcp.CallToolRequest) string {
	if req == nil || req.Extra == nil {
		return ""
	}
	return strings.TrimSpace(req.Extra.Header.Get(mcpsession.HeaderName))
}

func readMeInputSchema() map[string]any {
	return ReadMeInputSchema()
}

func ReadMeInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"modules": map[string]any{
				"type":        "array",
				"description": "Guidance modules to load.",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"diagram", "mockup", "interactive", "data_viz", "art", "chart", "elicitation"},
				},
			},
			"platform": map[string]any{
				"type":        "string",
				"enum":        []string{"mobile", "desktop", "unknown"},
				"description": "The client platform the widget will render on. Pass 'mobile' when your system prompt indicates a mobile client (narrow ~380px viewport) so SVG viewBox and layout guidance are sized accordingly; otherwise pass 'desktop'. Defaults to 'unknown' (desktop sizing).",
			},
		},
	}
}

func showWidgetInputSchema() map[string]any {
	return ShowWidgetInputSchema()
}

func ShowWidgetInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"loading_messages", "title", "widget_code"},
		"properties": map[string]any{
			"loading_messages": map[string]any{
				"type":        "array",
				"description": "One to four short status messages while rendering.",
				"items":       map[string]any{"type": "string"},
				"minItems":    1,
				"maxItems":    4,
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Short artifact title.",
			},
			"widget_code": map[string]any{
				"type":        "string",
				"description": "Raw SVG, HTML fragment, or full self-contained HTML document. For React artifacts, pass the bundled HTML generated by the web-artifacts-builder skill.",
			},
		},
	}
}
