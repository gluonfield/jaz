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

const DefaultGrokModel = "grok-4.5"

func sortReasoningEfforts(efforts []string) {
	sort.SliceStable(efforts, func(i, j int) bool {
		return reasoningEffortRank[efforts[i]] < reasoningEffortRank[efforts[j]]
	})
}

var (
	openAIModels = []Model{
		{Value: provider.OpenAIModelGPT56Sol, Label: "GPT-5.6 Sol", Description: "Frontier capability", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-sol"},
		{Value: provider.OpenAIModelGPT56Terra, Label: "GPT-5.6 Terra", Description: "Balanced capability and cost", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-terra"},
		{Value: provider.OpenAIModelGPT56Luna, Label: "GPT-5.6 Luna", Description: "Efficient high-volume workloads", ContextLength: 400000, OpenRouterID: "openai/gpt-5.6-luna"},
		{Value: "gpt-5.5", Label: "GPT-5.5", Description: "Previous frontier model", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.5"},
		{Value: provider.DefaultOpenAIModel, Label: "GPT-5.4 Mini", Description: "Fast and inexpensive", ContextLength: 400000, OpenRouterID: "openai/gpt-5.4-mini"},
		{Value: "gpt-5.3-codex-spark", Label: "GPT-5.3 Codex Spark", Description: "Tuned for coding", ContextLength: 400000},
	}
	agentModels = map[string][]Model{
		"codex": {
			{Value: provider.OpenAIModelGPT56Sol, Label: "GPT-5.6 Sol", Description: "Frontier capability", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-sol"},
			{Value: provider.OpenAIModelGPT56Terra, Label: "GPT-5.6 Terra", Description: "Balanced capability and cost", ContextLength: 1050000, OpenRouterID: "openai/gpt-5.6-terra"},
			{Value: provider.OpenAIModelGPT56Luna, Label: "GPT-5.6 Luna", Description: "Efficient high-volume workloads", ContextLength: 400000, OpenRouterID: "openai/gpt-5.6-luna"},
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
			openRouterAgentModel(provider.DefaultOpenRouterModel, "GLM 5.2", "Default OpenRouter coding model", 1048576),
			openRouterAgentModel("openai/"+provider.OpenAIModelGPT56Terra, "GPT-5.6 Terra", "Balanced capability and cost", 1050000),
			openRouterAgentModel("openai/"+provider.OpenAIModelGPT56Sol, "GPT-5.6 Sol", "Frontier capability", 1050000),
			openRouterAgentModel("openai/"+provider.OpenAIModelGPT56Luna, "GPT-5.6 Luna", "Efficient high-volume workloads", 400000),
			openRouterAgentModel("openai/gpt-5.4-mini", "GPT-5.4 Mini", "Fast and inexpensive", 400000),
			openRouterAgentModel("openai/gpt-5.5", "GPT-5.5", "Previous frontier model", 1050000),
			openRouterAgentModel("deepseek/deepseek-v4-flash", "DeepSeek V4 Flash", "Popular OpenRouter coding model", 1048576),
			openRouterAgentModel("xiaomi/mimo-v2.5", "MiMo-V2.5", "Popular OpenRouter coding model", 1048576),
			openRouterAgentModel("minimax/minimax-m3", "MiniMax M3", "Popular OpenRouter coding model", 1048576),
			openRouterAgentModel("deepseek/deepseek-v4-pro", "DeepSeek V4 Pro", "Popular OpenRouter coding model", 1048576),
			openRouterAgentModel("tencent/hy3-preview", "Hy3 preview", "Popular OpenRouter coding model", 262144),
			openRouterAgentModel("stepfun/step-3.7-flash", "Step 3.7 Flash", "Popular OpenRouter coding model", 256000),
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
	if model.Reasoning.Status == "" {
		model.Reasoning.Status = ReasoningUnavailable
		if model.OpenRouterID != "" {
			model.Reasoning.Status = ReasoningPending
		}
	}
	model.Reasoning.Efforts = cloneStrings(model.Reasoning.Efforts)
	if model.Pricing != nil {
		pricing := *model.Pricing
		model.Pricing = &pricing
	}
	return model
}

func openRouterAgentModel(value, label, description string, contextLength int) Model {
	return Model{Value: value, Label: label, Description: description, ContextLength: contextLength, OpenRouterID: value}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
