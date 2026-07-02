package modelcatalog

import "strings"

type Pricing struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

type Model struct {
	Value                  string   `json:"value"`
	Label                  string   `json:"label"`
	Description            string   `json:"description,omitempty"`
	ContextLength          int      `json:"context_length,omitempty"`
	Pricing                *Pricing `json:"pricing,omitempty"`
	OpenRouterID           string   `json:"openrouter_id,omitempty"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	ReasoningDefaultEffort string   `json:"reasoning_default_effort,omitempty"`
	ReasoningMandatory     bool     `json:"reasoning_mandatory,omitempty"`
}

var (
	openAIReasoningEfforts        = []string{"xhigh", "high", "medium", "low", "none"}
	codexReasoningEfforts         = []string{"minimal", "low", "medium", "high", "xhigh", "none"}
	claudeLongThinkingEfforts     = []string{"low", "medium", "high", "xhigh", "max", "ultracode"}
	claudeStandardThinkingEfforts = []string{"low", "medium", "high", "max"}
	noReasoningEfforts            = []string{}
	openAIModels                  = []Model{
		{Value: "gpt-5.5", Label: "GPT-5.5", Description: "Most capable", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.5", ReasoningEfforts: openAIReasoningEfforts},
		{Value: "gpt-5.4-mini", Label: "GPT-5.4 Mini", Description: "Fast and inexpensive", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4-mini", ReasoningEfforts: openAIReasoningEfforts},
		{Value: "gpt-5.3-codex-spark", Label: "GPT-5.3 Codex Spark", Description: "Tuned for coding", ContextLength: 400000, ReasoningEfforts: openAIReasoningEfforts},
	}
	agentModels = map[string][]Model{
		"codex": {
			{Value: "gpt-5.5", Label: "GPT-5.5", Description: "Most capable", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.5", ReasoningEfforts: codexReasoningEfforts},
			{Value: "gpt-5.3-codex-spark", Label: "GPT-5.3 Codex Spark", Description: "Account-gated research preview", ContextLength: 400000, ReasoningEfforts: codexReasoningEfforts},
			{Value: "gpt-5.4", Label: "GPT-5.4", Description: "Strong coding model", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4", ReasoningEfforts: codexReasoningEfforts},
			{Value: "gpt-5.4-mini", Label: "GPT-5.4 Mini", Description: "Fast and inexpensive", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4-mini", ReasoningEfforts: codexReasoningEfforts},
		},
		"claude": {
			{Value: "default", Label: "Default (Opus 4.8)", Description: "Opus 4.8 with 1M context · Recommended", ContextLength: 1000000, OpenRouterID: "anthropic/claude-opus-4.8", ReasoningEfforts: claudeLongThinkingEfforts},
			{Value: "claude-fable-5", Label: "Fable 5", Description: "Most capable for the hardest tasks", ContextLength: 1000000, OpenRouterID: "anthropic/claude-fable-5", ReasoningEfforts: claudeLongThinkingEfforts},
			{Value: "sonnet", Label: "Sonnet 5", Description: "Efficient for routine tasks", ContextLength: 200000, OpenRouterID: "anthropic/claude-sonnet-5", ReasoningEfforts: claudeStandardThinkingEfforts},
			{Value: "sonnet[1m]", Label: "Sonnet 5 (1M context)", Description: "Draws from usage credits", ContextLength: 1000000, OpenRouterID: "anthropic/claude-sonnet-5", ReasoningEfforts: claudeStandardThinkingEfforts},
			{Value: "haiku", Label: "Haiku 4.5", Description: "Fastest for quick answers", ContextLength: 200000, OpenRouterID: "anthropic/claude-haiku-4.5", ReasoningEfforts: noReasoningEfforts},
		},
		"grok": {
			{Value: "grok-build", Label: "Grok Build", Description: "Best for advanced coding tasks", ContextLength: 512000, OpenRouterID: "x-ai/grok-build-0.1"},
			{Value: "grok-composer-2.5-fast", Label: "Composer 2.5", Description: "Cursor's coding model", ContextLength: 200000},
		},
		"opencode": {
			{Value: "openrouter/openai/gpt-5.4-mini", Label: "GPT-5.4 Mini via OpenRouter", Description: "Fast and inexpensive", ContextLength: 400000},
			{Value: "openrouter/openai/gpt-5.5", Label: "GPT-5.5 via OpenRouter", Description: "Most capable", ContextLength: 1050000},
			{Value: "openai/gpt-5.4-mini", Label: "GPT-5.4 Mini via OpenAI", Description: "Direct OpenAI provider", ContextLength: 400000},
			{Value: "openai/gpt-5.5", Label: "GPT-5.5 via OpenAI", Description: "Direct OpenAI provider", ContextLength: 1050000},
		},
	}
)

func AgentModels(agent string) []Model {
	return cloneModels(agentModels[strings.ToLower(strings.TrimSpace(agent))])
}

func OpenAIModels() []Model {
	return cloneModels(openAIModels)
}

func Clone(models []Model) []Model {
	return cloneModels(models)
}

func cloneModels(models []Model) []Model {
	out := make([]Model, len(models))
	for i, model := range models {
		out[i] = model
		out[i].ReasoningEfforts = cloneStrings(model.ReasoningEfforts)
		if model.Pricing != nil {
			pricing := *model.Pricing
			out[i].Pricing = &pricing
		}
	}
	return out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
