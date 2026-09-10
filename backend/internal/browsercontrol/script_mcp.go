package browsercontrol

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolScript = "browser_js"

type ScriptInput struct {
	Code string `json:"code" jsonschema:"JavaScript using the persistent tab binding; empty code returns the browser API documentation"`
}

func (t directTools) Script(ctx context.Context, req *mcp.CallToolRequest, input ScriptInput) (*mcp.CallToolResult, ActionResult, error) {
	out, err := t.call(ctx, req, ActionInput{Action: ActionScript, Text: input.Code})
	return actionToolResult(out, err)
}
