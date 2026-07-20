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

type ReasoningStatus string

const (
	ReasoningUnavailable ReasoningStatus = "unavailable"
	ReasoningPending     ReasoningStatus = "pending"
	ReasoningReady       ReasoningStatus = "ready"
)

type Reasoning struct {
	Status        ReasoningStatus
	Efforts       []string
	DefaultEffort string
	Mandatory     bool
	Automatic     bool
}

type Model struct {
	Value         string    `json:"value"`
	Label         string    `json:"label"`
	Description   string    `json:"description,omitempty"`
	ContextLength int       `json:"context_length,omitempty"`
	Pricing       *Pricing  `json:"pricing,omitempty"`
	OpenRouterID  string    `json:"openrouter_id,omitempty"`
	Reasoning     Reasoning `json:"-"`
}

var reasoningEffortRank = map[string]int{
	"none": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5, "max": 6, "ultra": 7, "ultracode": 8,
}

const (
	DefaultGrokModel  = "grok-4.5"
	GrokComposerModel = "grok-composer-2.5-fast"
)

func sortReasoningEfforts(efforts []string) {
	sort.SliceStable(efforts, func(i, j int) bool {
		return reasoningEffortRank[efforts[i]] < reasoningEffortRank[efforts[j]]
	})
}

var (
	openAIModels = []Model{
		openRouterBackedModel(provider.OpenAIModelGPT56Sol, "GPT-5.6 Sol", "Frontier capability", 1050000, "openai/gpt-5.6-sol"),
		openRouterBackedModel(provider.OpenAIModelGPT56Terra, "GPT-5.6 Terra", "Balanced capability and cost", 1050000, "openai/gpt-5.6-terra"),
		openRouterBackedModel(provider.OpenAIModelGPT56Luna, "GPT-5.6 Luna", "Efficient high-volume workloads", 400000, "openai/gpt-5.6-luna"),
		openRouterBackedModel("gpt-5.5", "GPT-5.5", "Previous frontier model", 1050000, "openai/gpt-5.5"),
		openRouterBackedModel(provider.DefaultOpenAIModel, "GPT-5.4 Mini", "Fast and inexpensive", 400000, "openai/gpt-5.4-mini"),
		modelWithoutProviderReasoning("gpt-5.3-codex-spark", "GPT-5.3 Codex Spark", "Tuned for coding", 400000),
	}
	agentModels = map[string][]Model{
		"codex": {
			openRouterBackedModel(provider.OpenAIModelGPT56Sol, "GPT-5.6 Sol", "Frontier capability", 1050000, "openai/gpt-5.6-sol"),
			openRouterBackedModel(provider.OpenAIModelGPT56Terra, "GPT-5.6 Terra", "Balanced capability and cost", 1050000, "openai/gpt-5.6-terra"),
			openRouterBackedModel(provider.OpenAIModelGPT56Luna, "GPT-5.6 Luna", "Efficient high-volume workloads", 400000, "openai/gpt-5.6-luna"),
			modelWithoutProviderReasoning("gpt-5.3-codex-spark", "GPT-5.3 Codex Spark", "Account-gated research preview", 400000),
			openRouterBackedModel("gpt-5.5", "GPT-5.5", "Previous frontier model", 1050000, "openai/gpt-5.5"),
			openRouterBackedModel("gpt-5.4", "GPT-5.4", "Strong coding model", 400000, "openai/gpt-5.4"),
			openRouterBackedModel(provider.DefaultOpenAIModel, "GPT-5.4 Mini", "Fast and inexpensive", 400000, "openai/gpt-5.4-mini"),
		},
		"claude": {
			openRouterBackedModel("default", "Opus 4.8", "Recommended · 1M context", 1000000, "anthropic/claude-opus-4.8"),
			openRouterBackedModel("claude-fable-5[1m]", "Fable 5", "Most capable for the hardest tasks", 1000000, "anthropic/claude-fable-5"),
			openRouterBackedModel("sonnet", "Sonnet 5", "Efficient for routine tasks", 200000, "anthropic/claude-sonnet-5"),
			openRouterBackedModel("sonnet[1m]", "Sonnet 5 (1M context)", "Draws from usage credits", 1000000, "anthropic/claude-sonnet-5"),
			openRouterBackedModel("haiku", "Haiku 4.5", "Fastest for quick answers", 200000, "anthropic/claude-haiku-4.5"),
		},
		"grok": {
			modelWithoutProviderReasoning(DefaultGrokModel, "Grok 4.5", "Default Grok model", 512000),
			modelWithoutProviderReasoning(GrokComposerModel, "Composer 2.5", "Cursor's coding model", 200000),
		},
		"antigravity": {
			modelWithoutProviderReasoning("Gemini 3.5 Flash (Medium)", "Gemini 3.5 Flash", "Medium", 0),
			modelWithoutProviderReasoning("Gemini 3.5 Flash (High)", "Gemini 3.5 Flash", "High", 0),
			modelWithoutProviderReasoning("Gemini 3.5 Flash (Low)", "Gemini 3.5 Flash", "Low", 0),
			modelWithoutProviderReasoning("Gemini 3.1 Pro (Low)", "Gemini 3.1 Pro", "Low", 0),
			modelWithoutProviderReasoning("Gemini 3.1 Pro (High)", "Gemini 3.1 Pro", "High", 0),
			modelWithoutProviderReasoning("Claude Sonnet 4.6 (Thinking)", "Claude Sonnet 4.6", "Thinking", 0),
			modelWithoutProviderReasoning("Claude Opus 4.6 (Thinking)", "Claude Opus 4.6", "Thinking", 0),
			modelWithoutProviderReasoning("GPT-OSS 120B (Medium)", "GPT-OSS 120B", "Medium", 0),
		},
		"opencode": {
			openRouterNativeModel(provider.DefaultOpenRouterModel, "GLM 5.2", "Default OpenRouter coding model", 1048576),
			openRouterNativeModel("openai/"+provider.OpenAIModelGPT56Terra, "GPT-5.6 Terra", "Balanced capability and cost", 1050000),
			openRouterNativeModel("openai/"+provider.OpenAIModelGPT56Sol, "GPT-5.6 Sol", "Frontier capability", 1050000),
			openRouterNativeModel("openai/"+provider.OpenAIModelGPT56Luna, "GPT-5.6 Luna", "Efficient high-volume workloads", 400000),
			openRouterNativeModel("openai/gpt-5.4-mini", "GPT-5.4 Mini", "Fast and inexpensive", 400000),
			openRouterNativeModel("openai/gpt-5.5", "GPT-5.5", "Previous frontier model", 1050000),
			openRouterNativeModel("deepseek/deepseek-v4-flash", "DeepSeek V4 Flash", "Popular OpenRouter coding model", 1048576),
			openRouterNativeModel("xiaomi/mimo-v2.5", "MiMo-V2.5", "Popular OpenRouter coding model", 1048576),
			openRouterNativeModel("minimax/minimax-m3", "MiniMax M3", "Popular OpenRouter coding model", 1048576),
			openRouterNativeModel("deepseek/deepseek-v4-pro", "DeepSeek V4 Pro", "Popular OpenRouter coding model", 1048576),
			openRouterNativeModel("tencent/hy3-preview", "Hy3 preview", "Popular OpenRouter coding model", 262144),
			openRouterNativeModel("stepfun/step-3.7-flash", "Step 3.7 Flash", "Popular OpenRouter coding model", 256000),
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
	model.Reasoning.Efforts = cloneStrings(model.Reasoning.Efforts)
	if model.Pricing != nil {
		pricing := *model.Pricing
		model.Pricing = &pricing
	}
	return model
}

func modelWithoutProviderReasoning(value, label, description string, contextLength int) Model {
	return Model{Value: value, Label: label, Description: description, ContextLength: contextLength, Reasoning: Reasoning{Status: ReasoningUnavailable}}
}

func openRouterBackedModel(value, label, description string, contextLength int, openRouterID string) Model {
	model := modelWithoutProviderReasoning(value, label, description, contextLength)
	model.OpenRouterID = openRouterID
	model.Reasoning.Status = ReasoningPending
	return model
}

func openRouterNativeModel(value, label, description string, contextLength int) Model {
	return openRouterBackedModel(value, label, description, contextLength, value)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
