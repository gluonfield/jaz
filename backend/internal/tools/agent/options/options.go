package options

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/helpers"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Manager *acp.Manager
}

type input struct {
	Agent string `json:"agent,omitempty" jsonschema_description:"Optional ACP agent name to inspect, for example codex, claude, grok, opencode, or antigravity."`
	Name  string `json:"name,omitempty" jsonschema_description:"Optional case-insensitive model name or id filter. Use this for large provider catalogs such as OpenRouter."`
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function("agent_options", "List spawnable ACP agents and useful model choices. Call agent_options({}) for the default shortlist. For huge provider catalogs such as OpenRouter, pass agent and name to search model names or ids.", true, inputSchema())
}

func (t *Tool) Execute(_ context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[input](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	out, err := t.Manager.AgentOptions(acp.AgentOptionsRequest{Agent: req.Agent, Name: req.Name})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(out)
}

func inputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "Optional ACP agent name to inspect, for example codex, claude, grok, opencode, or antigravity.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Optional case-insensitive model name or id filter. Use this for large provider catalogs such as OpenRouter.",
			},
		},
		"required": []string{},
	}
}
