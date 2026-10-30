package visualize

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/visualizeguide"
)

const (
	ReadMeMCPToolName     = "visualise_read_me"
	ShowWidgetMCPToolName = "visualise_show_widget"

	MaxWidgetCodeBytes = 5 << 20
)

type Surface string

const (
	SurfaceChat   Surface = "chat"
	SurfaceWidget Surface = "widget"
	SurfaceCanvas Surface = "canvas"
)

func NormalizeSurface(surface string) Surface {
	switch Surface(strings.TrimSpace(surface)) {
	case SurfaceWidget, SurfaceCanvas:
		return Surface(strings.TrimSpace(surface))
	default:
		return SurfaceChat
	}
}

const (
	SavedMessage    = "Artifact saved in this conversation for display. Client rendering has not been confirmed."
	PreparedMessage = "Artifact prepared for display in the transcript. Client rendering has not been confirmed."
	DisplayReminder = "Do not duplicate the artifact in text or with another visualisation tool. Do not claim the user has already seen it."
)

type SessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type SessionEventPublisher interface {
	Publish(event sessionevents.Event)
}

type SessionSource interface {
	LoadSession(id string) (storage.Session, error)
}

type ReadMeInput struct {
	Modules  []string `json:"modules,omitempty" jsonschema:"diagram, mockup, interactive, data_viz, art, chart, elicitation"`
	Platform string   `json:"platform,omitempty"`
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
	Store    SessionEventAppender
	Sessions SessionSource
	Events   SessionEventPublisher
}

func NewMCPTools(store SessionEventAppender, sessions SessionSource, events SessionEventPublisher) *MCPTools {
	return &MCPTools{Store: store, Sessions: sessions, Events: events}
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	t.AddReadMeTo(server)
	t.AddShowWidgetTo(server)
}

func (t *MCPTools) AddReadMeTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ReadMeMCPToolName,
		Title:       "Read artifact guidance",
		Description: "Loads design-system and module guidance before creating a real SVG or HTML inline artifact. Call this silently before the first visual artifact in a turn.",
		InputSchema: readMeInputSchema(),
	}, t.ReadMe)
}

func (t *MCPTools) AddShowWidgetTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowWidgetMCPToolName,
		Title:       "Show inline artifact",
		Description: "Renders a finished SVG, HTML fragment, or full self-contained HTML document inline in the Jaz transcript. Do not use for placeholder or plumbing-demo widgets.",
		InputSchema: showWidgetInputSchema(),
	}, t.ShowWidget)
}

func (t *MCPTools) ReadMe(_ context.Context, _ *mcp.CallToolRequest, input ReadMeInput) (*mcp.CallToolResult, any, error) {
	return textResult(BuildReadMeGuide(input.Modules, input.Platform))
}

func (t *MCPTools) ShowWidget(_ context.Context, req *mcp.CallToolRequest, input ShowWidgetInput) (*mcp.CallToolResult, any, error) {
	_, artifact, err := BuildArtifact(input)
	if err != nil {
		return nil, nil, err
	}
	sessionID := mcpsession.SessionID(req)
	if sessionID == "" {
		return nil, nil, errors.New("artifact delivery requires a session")
	}
	session, err := t.Sessions.LoadSession(sessionID)
	if err != nil {
		return nil, nil, err
	}
	if !session.PresentsToUser() {
		return textResult("Artifact not displayed in the user's chat: this turn answers a bot or group. Use this turn's delivery channel to report the result.")
	}
	artifact.Presented = true
	events := []sessionevents.Event{{SessionID: sessionID, Type: sessionevents.TypeArtifact, Artifact: artifact}}
	if err := t.Store.AppendSessionEvents(sessionID, events...); err != nil {
		return nil, nil, err
	}
	if t.Events != nil {
		t.Events.Publish(events[0])
	}
	return textResult(SavedMessage, DisplayReminder)
}

// textResult builds a text-only MCP result with a deliberately nil structured
// output. Any handler whose Out type isn't `any` makes the SDK emit an
// outputSchema + structuredContent, which some clients surface in place of
// content — dropping the text. Returning through this helper keeps Out as `any`
// by construction, so text-only tools can't silently reintroduce that.
func textResult(texts ...string) (*mcp.CallToolResult, any, error) {
	content := make([]mcp.Content, len(texts))
	for i, text := range texts {
		content[i] = &mcp.TextContent{Text: text}
	}
	return &mcp.CallToolResult{Content: content}, nil, nil
}

// BuildReadMeGuide returns the design guidance scoped to the requested modules.
// The full guide overflows the agent's tool-result cap, so callers pass only the
// modules in play (e.g. "chart") and get the design-system core plus those
// sections. Structure lives in the visualizeguide template package.
func BuildReadMeGuide(modules []string, platform string) string {
	return visualizeguide.Render(visualizeguide.For(modules, platform))
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
				"description": "The client platform the widget will render on.",
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
