package modelcatalog

import (
	"sort"

	"github.com/wins/jaz/backend/internal/provider"
)

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

var reasoningEffortRank = map[string]int{
	"none": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5, "max": 6, "ultra": 7, "ultracode": 8,
}

const DefaultGrokModel = "grok-4.5"

func sortReasoningEfforts(efforts []string) {
	sort.SliceStable(efforts, func(i, j int) bool {
		return reasoningEffortRank[efforts[i]] < reasoningEffortRank[efforts[j]]
	})
}

func withUltracode(efforts []string) []string {
	hasXhigh := false
	for _, effort := range efforts {
		if effort == "ultracode" {
			return efforts
		}
		hasXhigh = hasXhigh || effort == "xhigh"
	}
	if !hasXhigh {
		return efforts
	}
	return append(efforts, "ultracode")
}

var (
	codexHarnessEfforts    = []string{"none", "minimal", "low", "medium", "high", "xhigh"}
	openAIReasoningEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
	codexUltraEfforts      = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}
	claudeHarnessEfforts   = []string{"low", "medium", "high", "xhigh", "max", "ultracode"}
	openAIModels           = []Model{
		{Value: provider.OpenAIModelGPT56Sol, Label: "GPT-5.6 Sol", Description: "Frontier capability", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-sol", ReasoningEfforts: openAIReasoningEfforts},
		{Value: provider.OpenAIModelGPT56Terra, Label: "GPT-5.6 Terra", Description: "Balanced capability and cost", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-terra", ReasoningEfforts: openAIReasoningEfforts},
		{Value: provider.OpenAIModelGPT56Luna, Label: "GPT-5.6 Luna", Description: "Efficient high-volume workloads", ContextLength: 400000, OpenRouterID: "openai/gpt-5.6-luna", ReasoningEfforts: openAIReasoningEfforts},
		{Value: "gpt-5.5", Label: "GPT-5.5", Description: "Previous frontier model", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.5"},
		{Value: provider.DefaultOpenAIModel, Label: "GPT-5.4 Mini", Description: "Fast and inexpensive", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4-mini"},
		{Value: "gpt-5.3-codex-spark", Label: "GPT-5.3 Codex Spark", Description: "Tuned for coding", ContextLength: 400000},
	}
	agentModels = map[string][]Model{
		"codex": {
			{Value: provider.OpenAIModelGPT56Sol, Label: "GPT-5.6 Sol", Description: "Frontier capability", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-sol", ReasoningEfforts: codexUltraEfforts},
			{Value: provider.OpenAIModelGPT56Terra, Label: "GPT-5.6 Terra", Description: "Balanced capability and cost", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-terra", ReasoningEfforts: codexUltraEfforts},
			{Value: provider.OpenAIModelGPT56Luna, Label: "GPT-5.6 Luna", Description: "Efficient high-volume workloads", ContextLength: 400000, OpenRouterID: "openai/gpt-5.6-luna", ReasoningEfforts: codexUltraEfforts},
			{Value: "gpt-5.3-codex-spark", Label: "GPT-5.3 Codex Spark", Description: "Account-gated research preview", ContextLength: 400000},
			{Value: "gpt-5.5", Label: "GPT-5.5", Description: "Previous frontier model", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.5"},
			{Value: "gpt-5.4", Label: "GPT-5.4", Description: "Strong coding model", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4"},
			{Value: provider.DefaultOpenAIModel, Label: "GPT-5.4 Mini", Description: "Fast and inexpensive", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4-mini"},
		},
		"claude": {
			{Value: "default", Label: "Opus 4.8", Description: "Recommended · 1M context", ContextLength: 1000000, OpenRouterID: "anthropic/claude-opus-4.8"},
			{Value: "claude-fable-5[1m]", Label: "Fable 5", Description: "Most capable for the hardest tasks", ContextLength: 1000000, OpenRouterID: "anthropic/claude-fable-5"},
			{Value: "sonnet", Label: "Sonnet 5", Description: "Efficient for routine tasks", ContextLength: 200000, OpenRouterID: "anthropic/claude-sonnet-5"},
			{Value: "sonnet[1m]", Label: "Sonnet 5 (1M context)", Description: "Draws from usage credits", ContextLength: 1000000, OpenRouterID: "anthropic/claude-sonnet-5"},
			{Value: "haiku", Label: "Haiku 4.5", Description: "Fastest for quick answers", ContextLength: 200000, OpenRouterID: "anthropic/claude-haiku-4.5"},
		},
		"grok": {
			{Value: DefaultGrokModel, Label: "Grok 4.5", Description: "Default Grok model", ContextLength: 512000},
			{Value: "grok-composer-2.5-fast", Label: "Composer 2.5", Description: "Cursor's coding model", ContextLength: 200000},
		},
		"antigravity": {
			{Value: "Gemini 3.5 Flash (Medium)", Label: "Gemini 3.5 Flash", Description: "Medium"},
			{Value: "Gemini 3.5 Flash (High)", Label: "Gemini 3.5 Flash", Description: "High"},
			{Value: "Gemini 3.5 Flash (Low)", Label: "Gemini 3.5 Flash", Description: "Low"},
			{Value: "Gemini 3.1 Pro (Low)", Label: "Gemini 3.1 Pro", Description: "Low"},
			{Value: "Gemini 3.1 Pro (High)", Label: "Gemini 3.1 Pro", Description: "High"},
			{Value: "Claude Sonnet 4.6 (Thinking)", Label: "Claude Sonnet 4.6", Description: "Thinking"},
			{Value: "Claude Opus 4.6 (Thinking)", Label: "Claude Opus 4.6", Description: "Thinking"},
			{Value: "GPT-OSS 120B (Medium)", Label: "GPT-OSS 120B", Description: "Medium"},
		},
		"opencode": {
			{Value: provider.DefaultOpenRouterModel, Label: "GLM 5.2", Description: "Default OpenRouter coding model", ContextLength: 1048576},
			{Value: "openai/" + provider.OpenAIModelGPT56Terra, Label: "GPT-5.6 Terra", Description: "Balanced capability and cost", ContextLength: 1050000},
			{Value: "openai/" + provider.OpenAIModelGPT56Sol, Label: "GPT-5.6 Sol", Description: "Frontier capability", ContextLength: 1050000},
			{Value: "openai/" + provider.OpenAIModelGPT56Luna, Label: "GPT-5.6 Luna", Description: "Efficient high-volume workloads", ContextLength: 400000},
			{Value: "openai/gpt-5.4-mini", Label: "GPT-5.4 Mini", Description: "Fast and inexpensive", ContextLength: 400000},
			{Value: "openai/gpt-5.5", Label: "GPT-5.5", Description: "Previous frontier model", ContextLength: 1050000},
			{Value: "deepseek/deepseek-v4-flash", Label: "DeepSeek V4 Flash", Description: "Popular OpenRouter coding model", ContextLength: 1048576},
			{Value: "xiaomi/mimo-v2.5", Label: "MiMo-V2.5", Description: "Popular OpenRouter coding model", ContextLength: 1048576},
			{Value: "minimax/minimax-m3", Label: "MiniMax M3", Description: "Popular OpenRouter coding model", ContextLength: 1048576},
			{Value: "deepseek/deepseek-v4-pro", Label: "DeepSeek V4 Pro", Description: "Popular OpenRouter coding model", ContextLength: 1048576},
			{Value: "tencent/hy3-preview", Label: "Hy3 preview", Description: "Popular OpenRouter coding model", ContextLength: 262144},
			{Value: "stepfun/step-3.7-flash", Label: "Step 3.7 Flash", Description: "Popular OpenRouter coding model", ContextLength: 256000},
		},
	}
)

func cloneModels(models []Model) []Model {
	out := make([]Model, len(models))
	for i, model := range models {
		out[i] = cloneModel(model)
	}
	return out
}

func cloneModel(model Model) Model {
	model.ReasoningEfforts = cloneStrings(model.ReasoningEfforts)
	if model.Pricing != nil {
		pricing := *model.Pricing
		model.Pricing = &pricing
	}
	return model
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
