package browsercontrol

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolHover = "browser_hover"
const ToolDrag = "browser_drag"

type DragInput struct {
	From string `json:"from" jsonschema:"source element ref from the latest page observation"`
	To   string `json:"to" jsonschema:"destination element ref from the same observation; both elements must be visible"`
}

func (t directTools) Hover(ctx context.Context, req *mcp.CallToolRequest, input RefInput) (*mcp.CallToolResult, ActionResult, error) {
	out, err := t.call(ctx, req, ActionInput{Action: ActionHover, Ref: input.Ref})
	return actionToolResult(out, err)
}

func (t directTools) Drag(ctx context.Context, req *mcp.CallToolRequest, input DragInput) (*mcp.CallToolResult, ActionResult, error) {
	out, err := t.call(ctx, req, ActionInput{Action: ActionDrag, Ref: input.From, Text: input.To})
	return actionToolResult(out, err)
}
