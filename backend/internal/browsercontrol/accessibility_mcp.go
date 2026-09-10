package browsercontrol

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolAXState = "browser_get_ax_state"

type AXStateInput struct {
	DisableDiffing bool `json:"disable_diffing,omitempty" jsonschema:"return the full accessibility tree instead of changes since the previous observation"`
}

func (t directTools) AXState(ctx context.Context, req *mcp.CallToolRequest, input AXStateInput) (*mcp.CallToolResult, ActionResult, error) {
	out, err := t.call(ctx, req, ActionInput{Action: ActionAXState, DisableDiffing: input.DisableDiffing})
	return actionToolResult(out, err)
}
