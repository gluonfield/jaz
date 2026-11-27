package visualize

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/visualizeguide"
)

const (
	GuideMCPToolName        = "artifact_guide"
	ShowArtifactMCPToolName = "show_artifact"
	ShowCanvasMCPToolName   = "show_canvas"
	GuideDescription        = "Returns the design rules for artifacts: the self-contained HTML or SVG that Jaz renders. Request the modules that fit the work: diagram, chart, mockup, interactive, art or elicitation. The canvas module instead returns this thread's editable canvas file, its format and how to present it. Call silently before the first artifact in a turn."
	ShowArtifactDescription = "Shows an artifact inline in the conversation: a chart, diagram, dashboard, calculator, mockup, illustration, widget or other interactive tool, as finished self-contained HTML or SVG, never a placeholder. Use it whenever a picture explains the answer better than text. The artifact stays in this thread; when the user wants to keep, publish, share or install it, call publish_app with the same html. Call artifact_guide before your first call."

	MaxArtifactBytes = 5 << 20
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
	SavedMessage    = "Artifact added to this conversation for display. Client rendering has not been confirmed."
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

type GuideInput struct {
	Modules  []string `json:"modules,omitempty"`
	Platform string   `json:"platform,omitempty"`
}

type ShowArtifactInput struct {
	LoadingMessages []string `json:"loading_messages"`
	Title           string   `json:"title"`
	HTML            string   `json:"html"`
}

type ShowArtifactOutput struct {
	Status       string `json:"status"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type"`
}

type Service struct {
	Store    SessionEventAppender
	Sessions SessionSource
	Events   SessionEventPublisher
	Canvases *canvas.Service
}

func NewService(store SessionEventAppender, sessions SessionSource, events SessionEventPublisher, canvases *canvas.Service) *Service {
	return &Service{Store: store, Sessions: sessions, Events: events, Canvases: canvases}
}

func (t *Service) AddTo(server *mcp.Server) {
	t.AddGuideTo(server)
	t.AddShowArtifactTo(server)
	t.AddCanvasTo(server)
}

func (t *Service) AddGuideTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        GuideMCPToolName,
		Title:       "Read artifact guidance",
		Description: GuideDescription,
		InputSchema: GuideInputSchema(),
	}, t.ReadGuide)
}

func (t *Service) AddShowArtifactTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowArtifactMCPToolName,
		Title:       "Show inline artifact",
		Description: ShowArtifactDescription,
		InputSchema: ShowArtifactInputSchema(),
	}, t.ShowArtifact)
}

func (t *Service) ReadGuide(_ context.Context, req *mcp.CallToolRequest, input GuideInput) (*mcp.CallToolResult, any, error) {
	guide, err := t.Guide(mcpsession.SessionID(req), input)
	if err != nil {
		return nil, nil, err
	}
	return textResult(guide)
}

func (t *Service) ShowArtifact(_ context.Context, req *mcp.CallToolRequest, input ShowArtifactInput) (*mcp.CallToolResult, any, error) {
	_, artifact, err := BuildArtifact(input)
	if err != nil {
		return nil, nil, err
	}
	return t.PresentMCP(mcpsession.SessionID(req), &sessionevents.PresentationEvent{Kind: sessionevents.PresentationArtifact, Artifact: artifact})
}

func (t *Service) PresentMCP(sessionID string, artifact *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error) {
	text, err := t.Present(sessionID, artifact)
	if err != nil {
		return nil, nil, err
	}
	return textResult(text...)
}

// Present shows an artifact in the user's thread and keeps it in the transcript.
func (t *Service) Present(sessionID string, artifact *sessionevents.PresentationEvent) ([]string, error) {
	if sessionID == "" {
		return nil, errors.New("artifact delivery requires a session")
	}
	session, err := t.Sessions.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	if !session.PresentsToUser() {
		return []string{"Artifact not displayed in the user's chat: this turn answers a bot or group. Use this turn's delivery channel to report the result."}, nil
	}
	artifact.Presented = true
	events := []sessionevents.Event{{SessionID: sessionID, Type: sessionevents.TypeArtifact, Artifact: artifact}}
	if err := t.Store.AppendSessionEvents(sessionID, events...); err != nil {
		return nil, err
	}
	if t.Events != nil {
		t.Events.Publish(events[0])
	}
	return []string{SavedMessage, DisplayReminder}, nil
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

// BuildGuide returns the design guidance scoped to the requested modules.
// The full guide overflows the agent's tool-result cap, so callers pass only the
// modules in play (e.g. "chart") and get the design-system core plus those
// sections. Structure lives in the visualizeguide template package.
func BuildGuide(modules []string, platform string) string {
	return visualizeguide.Render(visualizeguide.For(modules, platform))
}

func BuildArtifact(input ShowArtifactInput) (ShowArtifactOutput, *sessionevents.ArtifactEvent, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return ShowArtifactOutput{}, nil, errors.New("title is required")
	}
	code := input.HTML
	if strings.TrimSpace(code) == "" {
		return ShowArtifactOutput{}, nil, errors.New("html is required")
	}
	codeBytes := len([]byte(code))
	if codeBytes > MaxArtifactBytes {
		return ShowArtifactOutput{}, nil, fmt.Errorf("html exceeds %d bytes", MaxArtifactBytes)
	}
	messages, err := normalizeLoadingMessages(input.LoadingMessages)
	if err != nil {
		return ShowArtifactOutput{}, nil, err
	}
	kind := ArtifactKind(code)
	output := ShowArtifactOutput{
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

func GuideInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"modules": map[string]any{
				"type":        "array",
				"description": "Guidance modules to load.",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"diagram", "mockup", "interactive", "data_viz", "art", "chart", "elicitation", "canvas"},
				},
			},
			"platform": map[string]any{
				"type":        "string",
				"description": "The client platform the artifact will render on: mobile when the prompt says Device: Mobile, otherwise desktop.",
			},
		},
	}
}

func ShowArtifactInputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"loading_messages", "title", "html"},
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
			"html": map[string]any{
				"type":        "string",
				"description": "Raw SVG, HTML fragment, or full self-contained HTML document. For React artifacts, pass the bundled HTML generated by the web-artifacts-builder skill.",
			},
		},
	}
}
