package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/internal/toolsearch"
)

const (
	toolSearchToolName     = "tool_search"
	toolSearchDefaultLimit = 8
)

type toolExposure struct {
	direct      []tools.Definition
	deferred    []searchableTool
	deferredBy  map[string]searchableTool
	exposed     map[string]bool
	pending     map[string]bool
	searchIndex toolsearch.Index
}

type searchableTool struct {
	name       string
	definition tools.Definition
	output     loadableToolSpec
}

type loadableToolSpec struct {
	Type         string         `json:"type"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Strict       bool           `json:"strict"`
	DeferLoading bool           `json:"defer_loading"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

type toolSearchOutput struct {
	Status    string             `json:"status"`
	Execution string             `json:"execution"`
	Tools     []loadableToolSpec `json:"tools"`
}

func newToolExposure(definitions []tools.Definition, messages []provider.Message, deferTool func(string) bool) (*toolExposure, error) {
	exposure := &toolExposure{
		deferredBy: map[string]searchableTool{},
		exposed:    map[string]bool{},
		pending:    map[string]bool{},
	}
	var documents []toolsearch.Document
	for _, def := range definitions {
		name := tools.DefinitionName(def)
		if name == "" {
			exposure.direct = append(exposure.direct, def)
			continue
		}
		if name == toolSearchToolName {
			continue
		}
		if deferTool != nil && deferTool(name) {
			entry, err := newSearchableTool(def)
			if err != nil {
				return nil, err
			}
			exposure.deferred = append(exposure.deferred, entry)
			exposure.deferredBy[name] = entry
			documents = append(documents, toolsearch.Document{Name: name, Description: entry.output.Description, Parameters: entry.output.Parameters})
			continue
		}
		exposure.direct = append(exposure.direct, def)
	}
	if len(exposure.deferred) > 0 {
		exposure.searchIndex = toolsearch.New(documents)
		exposure.direct = append(exposure.direct, toolSearchDefinition())
	}
	exposure.restoreSearchResults(messages)
	return exposure, nil
}

func (e *toolExposure) definitions() []tools.Definition {
	out := append([]tools.Definition(nil), e.direct...)
	for _, entry := range e.deferred {
		if e.exposed[entry.name] {
			out = append(out, entry.definition)
		}
	}
	return out
}

func (e *toolExposure) isDeferred(name string) bool {
	_, ok := e.deferredBy[name]
	return ok
}

func (e *toolExposure) isExposed(name string) bool {
	return e.exposed[name]
}

func (e *toolExposure) executeSearch(call provider.ToolCall) tools.Result {
	inputs, err := toolCallInputs(call)
	if err != nil {
		return tools.Result{Content: marshalToolError(err.Error())}
	}
	query := strings.TrimSpace(tools.StringInput(inputs, "query"))
	if query == "" {
		return tools.Result{Content: marshalToolError("query must not be empty")}
	}
	limit := tools.IntInput(inputs, "limit", toolSearchDefaultLimit)
	if limit <= 0 {
		return tools.Result{Content: marshalToolError("limit must be greater than zero")}
	}
	ranked := e.searchIndex.Rank(query)
	results := ranked[:min(limit, len(ranked))]
	outputs := make([]loadableToolSpec, 0, len(results))
	for _, position := range results {
		result := e.deferred[position]
		e.pending[result.name] = true
		outputs = append(outputs, result.output)
	}
	content, err := json.Marshal(toolSearchOutput{Status: "completed", Execution: "client", Tools: outputs})
	if err != nil {
		return tools.Result{Content: marshalToolError(err.Error())}
	}
	return tools.Result{Content: string(content)}
}

func (e *toolExposure) restoreSearchResults(messages []provider.Message) {
	toolCalls := map[string]string{}
	for _, msg := range messages {
		switch provider.MessageRole(msg) {
		case "assistant":
			for _, call := range provider.MessageToolCalls(msg) {
				toolCalls[provider.ToolCallID(call)] = provider.ToolCallName(call)
			}
		case "tool":
			if toolCalls[provider.MessageToolCallID(msg)] == toolSearchToolName {
				e.exposeSearchOutput(provider.MessageContent(msg))
			}
		}
	}
}

func (e *toolExposure) exposeSearchOutput(content string) {
	var output toolSearchOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil || output.Status != "completed" {
		return
	}
	for _, tool := range output.Tools {
		if e.isDeferred(tool.Name) {
			e.exposed[tool.Name] = true
		}
	}
}

func (e *toolExposure) commitSearchResults() {
	for name := range e.pending {
		e.exposed[name] = true
		delete(e.pending, name)
	}
}

func newSearchableTool(def tools.Definition) (searchableTool, error) {
	fn := def.GetFunction()
	if fn == nil || strings.TrimSpace(fn.Name) == "" {
		return searchableTool{}, fmt.Errorf("deferred tool %q is not a named function tool", tools.DefinitionName(def))
	}
	output := loadableToolSpec{
		Type:         "function",
		Name:         fn.Name,
		Description:  fn.Description.Or(""),
		Strict:       fn.Strict.Or(false),
		DeferLoading: true,
		Parameters:   map[string]any(fn.Parameters),
	}
	return searchableTool{name: output.Name, definition: def, output: output}, nil
}

func toolSearchDefinition() tools.Definition {
	description := fmt.Sprintf(`# Tool discovery

Searches over deferred tool metadata with BM25 and exposes matching tools for the next model call.

You have access to tools from the following sources:
- MCP: user-connected MCP servers.
Some of the tools may not have been provided to you upfront, and you should use this tool (%s) to search for the required tools. For MCP tool discovery, always use %s instead of list_mcp_resources or list_mcp_resource_templates.`, toolSearchToolName, toolSearchToolName)
	return tools.Function(toolSearchToolName, description, false, tools.ObjectSchema(map[string]any{
		"query": tools.StringSchema("Search query for deferred tools."),
		"limit": tools.NumberSchema(fmt.Sprintf("Maximum number of tools to return. Defaults to %d.", toolSearchDefaultLimit)),
	}, []string{"query"}))
}
