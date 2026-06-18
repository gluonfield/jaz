package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/kljensen/snowball/english"
	unidecode "github.com/mozillazg/go-unidecode"
	"github.com/rivo/uniseg"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/tools"
)

const (
	toolSearchToolName     = "tool_search"
	toolSearchDefaultLimit = 8
)

var toolSearchStopwords = newToolSearchStopwords()

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
	output     loadableToolSpec
	termCounts map[string]int
	termTotal  int
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
			exposure.averageTerms += float64(entry.termTotal)
			continue
		}
		exposure.direct = append(exposure.direct, def)
	}
	if len(exposure.deferred) > 0 {
		exposure.averageTerms /= float64(len(exposure.deferred))
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
	results := e.search(query, limit)
	outputs := make([]loadableToolSpec, 0, len(results))
	for _, result := range results {
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

func (e *toolExposure) search(query string, limit int) []searchableTool {
	queryTerms := tokenizeToolSearch(query)
	if len(queryTerms) == 0 || len(e.deferred) == 0 {
		return nil
	}
	documentFrequency := map[string]int{}
	for _, term := range uniqueTerms(queryTerms) {
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
	name := output.Name
	parts := []string{name, strings.ReplaceAll(name, "_", " "), output.Description}
	appendSchemaSearchText(output.Parameters, &parts)
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
	text = strings.ToLower(unidecode.Unidecode(text))
	tokens := []string{}
	for state := -1; text != ""; {
		word, rest, newState := uniseg.FirstWordInString(text, state)
		state = newState
		if rest == text {
			break
		}
		text = rest
		if !toolSearchWordToken(word) || toolSearchStopwords[word] {
			continue
		}
		if stem := english.Stem(word, false); stem != "" {
			tokens = append(tokens, stem)
		}
	}
	return tokens
}

func toolSearchWordToken(token string) bool {
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
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

func newToolSearchStopwords() map[string]bool {
	words := strings.Fields(`i me my myself we our ours ourselves you you're you've you'll you'd your yours yourself yourselves he him his himself she she's her hers herself it it's its itself they them their theirs themselves what which who whom this that that'll these those am is are was were be been being have has had having do does did doing a an the and but if or because as until while of at by for with about against between into through during before after above below to from up down in out on off over under again further then once here there when where why how all any both each few more most other some such no nor not only own same so than too very s t can will just don don't should should've now d ll m o re ve y ain aren aren't couldn couldn't didn didn't doesn doesn't hadn hadn't hasn hasn't haven haven't isn isn't ma mightn mightn't mustn mustn't needn needn't shan shan't shouldn shouldn't wasn wasn't weren weren't won won't wouldn wouldn't`)
	stopwords := make(map[string]bool, len(words))
	for _, word := range words {
		stopwords[word] = true
	}
	return stopwords
}
