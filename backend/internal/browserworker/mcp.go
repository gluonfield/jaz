package browserworker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const ToolName = "browser"

type Backend interface {
	Call(context.Context, ActionInput) (ActionOutput, error)
}

type UnavailableBackend struct{}

type ActionInput struct {
	Action   string `json:"action" jsonschema:"navigate, snapshot, state, screenshot, click, type, fill, press, hover, scroll, select, wait, tabs, pdf, status"`
	URL      string `json:"url,omitempty" jsonschema:"target URL for navigate"`
	Selector string `json:"selector,omitempty" jsonschema:"stable selector or accessibility locator when an action targets an element"`
	Text     string `json:"text,omitempty" jsonschema:"text for type/fill/select/wait actions"`
	Key      string `json:"key,omitempty" jsonschema:"keyboard key for press actions"`
	Amount   int    `json:"amount,omitempty" jsonschema:"scroll amount in CSS pixels; wait timeout in milliseconds"`
	Session  string `json:"-"`
}

type ActionOutput struct {
	Status          string          `json:"status"`
	Text            string          `json:"text,omitempty"`
	ImageBase64     string          `json:"-"`
	ImageMIMEType   string          `json:"-"`
	PDFBase64       string          `json:"-"`
	PDFBase64Length int             `json:"pdf_base64_length,omitempty"`
	Data            json.RawMessage `json:"-"`
}

func AddMCPTools(server *mcp.Server, backend Backend) {
	if backend == nil {
		backend = UnavailableBackend{}
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolName,
		Title:       "Use browser",
		Description: "Operate the delegated Jaz browser session. Prefer snapshot/status before screenshots; use screenshots only when visual state matters.",
		InputSchema: actionSchema(),
	}, tool{backend: backend}.Call)
}

func RemoveMCPTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(ToolName)
	}
}

type tool struct {
	backend Backend
}

func (t tool) Call(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, ActionOutput, error) {
	action := strings.TrimSpace(input.Action)
	if action == "" {
		return nil, ActionOutput{}, errors.New("action is required")
	}
	input.Action = action
	input.Session = mcpsession.SessionID(req)
	out, err := t.backend.Call(ctx, input)
	if err != nil {
		return nil, ActionOutput{}, err
	}
	return contentResult(out), out, nil
}

func (UnavailableBackend) Call(context.Context, ActionInput) (ActionOutput, error) {
	return ActionOutput{}, errors.New("browser backend is not connected; configure a managed browser or client extension bridge")
}

func contentResult(out ActionOutput) *mcp.CallToolResult {
	var content []mcp.Content
	if text := strings.TrimSpace(out.Text); text != "" {
		content = append(content, &mcp.TextContent{Text: text})
	}
	if out.ImageBase64 != "" {
		mimeType := strings.TrimSpace(out.ImageMIMEType)
		if mimeType == "" {
			mimeType = "image/png"
		}
		content = append(content, &mcp.ImageContent{Data: []byte(out.ImageBase64), MIMEType: mimeType})
	}
	if out.PDFBase64 != "" {
		if data, err := base64.StdEncoding.DecodeString(out.PDFBase64); err == nil {
			content = append(content, &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{
				URI:      "jaz://browser/output.pdf",
				MIMEType: "application/pdf",
				Blob:     data,
			}})
		}
	}
	if len(content) == 0 {
		return nil
	}
	return &mcp.CallToolResult{Content: content}
}

func actionSchema() map[string]any {
	actions := append(SupportedExtensionActions(), ActionPDF)
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"action"},
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Browser operation to perform.",
				"enum":        actions,
			},
			"url": map[string]any{
				"type":        "string",
				"description": "Target URL for navigate.",
			},
			"selector": map[string]any{
				"type":        "string",
				"description": "Stable selector or accessibility locator when an action targets an element.",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Text for type, fill, select, or wait actions.",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "Keyboard key for press actions.",
			},
			"amount": map[string]any{
				"type":        "integer",
				"description": "Scroll amount in CSS pixels, or wait timeout in milliseconds.",
			},
		},
	}
}
