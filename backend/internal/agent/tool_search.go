package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
)

const (
	toolSearchToolName     = "tool_search"
	toolSearchDefaultLimit = 8
)

var toolSearchTokenPattern = regexp.MustCompile(`[A-Za-z0-9]+`)

type toolExposure struct {
	direct       []tools.Definition
	deferred     []searchableTool
	deferredBy   map[string]searchableTool
	exposed      map[string]bool
	pending      map[string]bool
	averageTerms float64
}

type searchableTool struct {
	name       string
	definition tools.Definition
	output     map[string]any
	termCounts map[string]int
	termTotal  int
}

func newToolExposure(definitions []tools.Definition, deferTool func(string) bool) *toolExposure {
	exposure := &toolExposure{
		deferredBy: map[string]searchableTool{},
		exposed:    map[string]bool{},
		pending:    map[string]bool{},
	}
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
			if err == nil {
				exposure.deferred = append(exposure.deferred, entry)
				exposure.deferredBy[name] = entry
				exposure.averageTerms += float64(entry.termTotal)
			}
			continue
		}
		exposure.direct = append(exposure.direct, def)
	}
	if len(exposure.deferred) > 0 {
		exposure.averageTerms /= float64(len(exposure.deferred))
		exposure.direct = append(exposure.direct, toolSearchDefinition())
	}
	return exposure
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
	results := e.search(query, limit)
	outputs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		e.pending[result.name] = true
		outputs = append(outputs, result.output)
	}
	content, err := json.Marshal(map[string]any{
		"status":    "completed",
		"execution": "client",
		"tools":     outputs,
	})
	if err != nil {
		return tools.Result{Content: marshalToolError(err.Error())}
	}
	return tools.Result{Content: string(content)}
}

func (e *toolExposure) commitSearchResults() {
	for name := range e.pending {
		e.exposed[name] = true
		delete(e.pending, name)
	}
}

func (e *toolExposure) search(query string, limit int) []searchableTool {
	queryTerms := uniqueTerms(tokenizeToolSearch(query))
	if len(queryTerms) == 0 || len(e.deferred) == 0 {
		return nil
	}
	documentFrequency := map[string]int{}
	for _, term := range queryTerms {
		for _, entry := range e.deferred {
			if entry.termCounts[term] > 0 {
				documentFrequency[term]++
			}
		}
	}
	type scoredTool struct {
		searchableTool
		score float64
	}
	var scored []scoredTool
	for _, entry := range e.deferred {
		score := e.bm25Score(entry, queryTerms, documentFrequency)
		if score > 0 {
			scored = append(scored, scoredTool{searchableTool: entry, score: score})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].name < scored[j].name
		}
		return scored[i].score > scored[j].score
	})
	if limit > len(scored) {
		limit = len(scored)
	}
	out := make([]searchableTool, 0, limit)
	for _, result := range scored[:limit] {
		out = append(out, result.searchableTool)
	}
	return out
}

func (e *toolExposure) bm25Score(entry searchableTool, queryTerms []string, documentFrequency map[string]int) float64 {
	const (
		k1 = 1.2
		b  = 0.75
	)
	if entry.termTotal == 0 || e.averageTerms == 0 {
		return 0
	}
	n := float64(len(e.deferred))
	lengthFactor := k1 * (1 - b + b*float64(entry.termTotal)/e.averageTerms)
	var score float64
	for _, term := range queryTerms {
		tf := float64(entry.termCounts[term])
		if tf == 0 {
			continue
		}
		df := float64(documentFrequency[term])
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		score += idf * (tf * (k1 + 1)) / (tf + lengthFactor)
	}
	return score
}

func newSearchableTool(def tools.Definition) (searchableTool, error) {
	output, err := loadableToolSpec(def)
	if err != nil {
		return searchableTool{}, err
	}
	name, _ := output["name"].(string)
	description, _ := output["description"].(string)
	parts := []string{name, strings.ReplaceAll(name, "_", " "), description}
	appendSchemaSearchText(output["parameters"], &parts)
	tokens := tokenizeToolSearch(strings.Join(parts, " "))
	return searchableTool{
		name:       name,
		definition: def,
		output:     output,
		termCounts: termCounts(tokens),
		termTotal:  len(tokens),
	}, nil
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

func loadableToolSpec(def tools.Definition) (map[string]any, error) {
	encoded, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return nil, err
	}
	fn, ok := raw["function"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tool %q is not a function tool", tools.DefinitionName(def))
	}
	out := map[string]any{
		"type":          "function",
		"name":          fn["name"],
		"defer_loading": true,
	}
	for _, key := range []string{"description", "strict", "parameters"} {
		if value, ok := fn[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func appendSchemaSearchText(value any, parts *[]string) {
	switch schema := value.(type) {
	case map[string]any:
		if description, ok := schema["description"].(string); ok {
			*parts = append(*parts, description)
		}
		if properties, ok := schema["properties"].(map[string]any); ok {
			for name, child := range properties {
				*parts = append(*parts, name)
				appendSchemaSearchText(child, parts)
			}
		}
		appendSchemaSearchText(schema["items"], parts)
		appendSchemaSearchText(schema["anyOf"], parts)
	case []any:
		for _, child := range schema {
			appendSchemaSearchText(child, parts)
		}
	}
}

func tokenizeToolSearch(text string) []string {
	matches := toolSearchTokenPattern.FindAllString(strings.ToLower(text), -1)
	tokens := make([]string, 0, len(matches))
	for _, match := range matches {
		if match != "" {
			tokens = append(tokens, match)
		}
	}
	return tokens
}

func termCounts(tokens []string) map[string]int {
	counts := map[string]int{}
	for _, token := range tokens {
		counts[token]++
	}
	return counts
}

func uniqueTerms(tokens []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}
