package boards

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const (
	PublishMCPToolName = "publish_app"
	PublishDescription = "Publishes this loop run's board app: the app this loop keeps up to date as a tile on the user's Jaz board. Write the HTML fragment to the app file first, then call this; alternatively pass the fragment inline via html. Invalid fragments fail; host-rendering warnings are advisory."
)

type MCPPublisher interface {
	PublishForSession(ctx context.Context, sessionID string, input PublishInput) (Published, error)
}

type MCPPublishInput struct {
	Title    string `json:"title,omitempty"`
	SizeHint string `json:"size_hint,omitempty"`
	HTML     string `json:"html,omitempty"`
}

// MCPPublishOutput carries the keys every publish_app result has, so clients
// show a routine's app with the card they show for any other.
type MCPPublishOutput struct {
	Note     string   `json:"note"`
	AppID    string   `json:"app_id"`
	Title    string   `json:"title"`
	Kind     string   `json:"kind"`
	Version  int      `json:"version"`
	SizeHint string   `json:"size_hint,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func PublishOutput(published Published) MCPPublishOutput {
	note := "Saved and shown on the board."
	if len(published.Warnings) > 0 {
		note = "Saved and shown on the board. Fix the host-rendering warnings and publish again."
	}
	return MCPPublishOutput{
		Note:     note,
		AppID:    published.App.ID,
		Title:    published.App.Title,
		Kind:     published.App.Kind,
		Version:  published.App.CurrentVersion,
		SizeHint: published.SizeHint,
		Warnings: published.Warnings,
	}
}

func AddMCPTools(server *mcp.Server, publisher MCPPublisher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        PublishMCPToolName,
		Title:       "Publish board app",
		Description: PublishDescription,
		InputSchema: PublishInputSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPPublishInput) (*mcp.CallToolResult, MCPPublishOutput, error) {
		out, err := publish(ctx, publisher, req, input)
		return nil, out, err
	})
}

func publish(ctx context.Context, publisher MCPPublisher, req *mcp.CallToolRequest, input MCPPublishInput) (MCPPublishOutput, error) {
	sessionID := mcpsession.SessionID(req)
	if sessionID == "" {
		return MCPPublishOutput{}, fmt.Errorf("%s requires %s", PublishMCPToolName, mcpsession.HeaderName)
	}
	published, err := publisher.PublishForSession(ctx, sessionID, PublishInput(input))
	if err != nil {
		return MCPPublishOutput{}, err
	}
	return PublishOutput(published), nil
}

func PublishInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "App title shown on the tile header and in Apps. Defaults to the loop name.",
			},
			"size_hint": map[string]any{
				"type":        "string",
				"description": "Proposed tile size as WxH (W 1-6, H 1-8), e.g. 2x2. The user can resize afterwards.",
			},
			"html": map[string]any{
				"type":        "string",
				"description": "The app as an HTML fragment. When omitted, the loop's app file is read instead.",
			},
		},
	}
}
