package widgets

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const (
	PublishMCPToolName = "visualise:publish_widget"
	PublishToolName    = "visualise_publish_widget"
)

type MCPPublisher interface {
	PublishForSession(sessionID string, input PublishInput) (Widget, []string, error)
}

type MCPPublishInput struct {
	Title    string `json:"title,omitempty"`
	SizeHint string `json:"size_hint,omitempty"`
	HTML     string `json:"html,omitempty"`
}

type MCPPublishOutput struct {
	WidgetID string   `json:"widget_id"`
	Title    string   `json:"title"`
	Version  int      `json:"version"`
	SizeHint string   `json:"size_hint,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func AddMCPTools(server *mcp.Server, publisher MCPPublisher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        PublishMCPToolName,
		Title:       "Publish board widget",
		Description: "Publishes this loop run's Jaz board widget. Write the HTML fragment to widget/index.html first, then call this; alternatively pass the fragment inline via html. Returns validation errors and non-fatal lint warnings so you can fix and retry within the run.",
		InputSchema: PublishInputSchema(),
	}, publishMCP(publisher))
}

func publishMCP(publisher MCPPublisher) func(context.Context, *mcp.CallToolRequest, MCPPublishInput) (*mcp.CallToolResult, MCPPublishOutput, error) {
	return func(_ context.Context, req *mcp.CallToolRequest, input MCPPublishInput) (*mcp.CallToolResult, MCPPublishOutput, error) {
		return publish(publisher, req, input)
	}
}

func publish(publisher MCPPublisher, req *mcp.CallToolRequest, input MCPPublishInput) (*mcp.CallToolResult, MCPPublishOutput, error) {
	sessionID := sessionIDFromRequest(req)
	if sessionID == "" {
		return nil, MCPPublishOutput{}, fmt.Errorf("%s requires %s", PublishMCPToolName, mcpsession.HeaderName)
	}
	widget, warnings, err := publisher.PublishForSession(sessionID, PublishInput{
		Title:    input.Title,
		SizeHint: input.SizeHint,
		HTML:     input.HTML,
	})
	if err != nil {
		return nil, MCPPublishOutput{}, err
	}
	out := MCPPublishOutput{
		WidgetID: widget.ID,
		Title:    widget.Title,
		Version:  widget.CurrentVersion,
		SizeHint: widget.SizeHint,
		Warnings: warnings,
	}
	content := fmt.Sprintf("Published widget %q version %d.", widget.Title, widget.CurrentVersion)
	if widget.SizeHint != "" {
		content = fmt.Sprintf("Published widget %q version %d (size %s).", widget.Title, widget.CurrentVersion, widget.SizeHint)
	}
	if len(warnings) > 0 {
		content += "\n\nLint warnings (published anyway — fix them and republish in this run):\n- " + strings.Join(warnings, "\n- ")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: content}}}, out, nil
}

func sessionIDFromRequest(req *mcp.CallToolRequest) string {
	if req.Extra == nil {
		return ""
	}
	return strings.TrimSpace(req.Extra.Header.Get(mcpsession.HeaderName))
}

func PublishInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Widget title shown on the tile header. Defaults to the loop name.",
			},
			"size_hint": map[string]any{
				"type":        "string",
				"description": "Proposed tile size as WxH (W 1-6, H 1-8), e.g. 2x2. The user can resize afterwards.",
			},
			"html": map[string]any{
				"type":        "string",
				"description": "Widget HTML fragment. When omitted, the loop's widget/index.html file is read instead.",
			},
		},
	}
}
