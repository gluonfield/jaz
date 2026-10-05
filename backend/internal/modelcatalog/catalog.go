package modelcatalog

import (
	"sort"
	"strings"

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
	Value           string    `json:"value"`
	Label           string    `json:"label"`
	Description     string    `json:"description,omitempty"`
	ContextLength   int       `json:"context_length,omitempty"`
	InputModalities []string  `json:"input_modalities,omitempty"`
	Pricing         *Pricing  `json:"pricing,omitempty"`
	OpenRouterID    string    `json:"openrouter_id,omitempty"`
	Reasoning       Reasoning `json:"-"`
}

type knownModelCapabilities struct {
	InputModalities []string
}

var reasoningEffortRank = map[string]int{
	"none": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5, "max": 6, "ultra": 7, "ultracode": 8,
}

func sortReasoningEfforts(efforts []string) {
	sort.SliceStable(efforts, func(i, j int) bool {
		return reasoningEffortRank[efforts[i]] < reasoningEffortRank[efforts[j]]
	})
}

var (
	knownModels = map[string]knownModelCapabilities{
		"qwen3.8-max-preview": {
			InputModalities: []string{"text", "image"},
		},
	}
	openAIModels = []Model{
		openRouterBackedModel(provider.OpenAIModelGPT6Astra, "GPT-6 Astra", "Frontier intelligence for the most demanding work", "openai/gpt-6-astra"),
		openRouterBackedModel(provider.OpenAIModelGPT61Sol, "GPT-6.1 Sol", "Latest workhorse for coding and everyday work", "openai/gpt-6.1-sol"),
		openRouterBackedModel(provider.OpenAIModelGPT6Sol, "GPT-6 Sol", "Previous generation workhorse", "openai/gpt-6-sol"),
		openRouterBackedModel(provider.DefaultOpenAIModel, "GPT-6 Luna", "Fast and affordable for easier tasks", "openai/gpt-6-luna"),
	}
	agentModels = map[string][]Model{
		"codex": openAIModels,
		"claude": {
			modelWithoutProviderReasoning("opus[1m]", "Opus 5.5", "Recommended"),
			openRouterBackedModel("fable[1m]", "Fable 5.1", "Most capable for the hardest tasks", "anthropic/claude-fable-5.1"),
			openRouterBackedModel("claude-fable-5[1m]", "Fable 5", "Previous Fable model", "anthropic/claude-fable-5"),
			openRouterBackedModel("sonnet", "Sonnet 5.5", "Efficient for routine tasks", "anthropic/claude-sonnet-5.5"),
			openRouterBackedModel("haiku", "Haiku 4.5", "Fastest for quick answers", "anthropic/claude-haiku-4.5"),
		},
		"grok": {
			modelWithoutProviderReasoning("grok-4.7", "Grok 4.7", "Latest frontier model"),
			modelWithoutProviderReasoning("grok-4.7-build-fast", "Grok 4.7 Fast", "Fast variant at twice the price"),
			modelWithoutProviderReasoning("grok-4.6", "Grok 4.6", "Previous Grok model"),
			modelWithoutProviderReasoning("grok-4.5", "Grok 4.5", "Older Grok model"),
		},
		"opencode": {
			openRouterNativeModel(provider.DefaultOpenRouterModel, "GLM 5.2", "Default OpenRouter coding model"),
			openRouterNativeModel("openai/"+provider.OpenAIModelGPT6Astra, "GPT-6 Astra", "Frontier intelligence for the most demanding work"),
			openRouterNativeModel("openai/"+provider.OpenAIModelGPT61Sol, "GPT-6.1 Sol", "Latest workhorse for coding and everyday work"),
			openRouterNativeModel("openai/"+provider.OpenAIModelGPT6Sol, "GPT-6 Sol", "Previous generation workhorse"),
			openRouterNativeModel("openai/"+provider.DefaultOpenAIModel, "GPT-6 Luna", "Fast and affordable for easier tasks"),
			openRouterNativeModel("deepseek/deepseek-v4-flash", "DeepSeek V4 Flash", "Popular OpenRouter coding model"),
			openRouterNativeModel("xiaomi/mimo-v2.5", "MiMo-V2.5", "Popular OpenRouter coding model"),
			openRouterNativeModel("minimax/minimax-m3", "MiniMax M3", "Popular OpenRouter coding model"),
			openRouterNativeModel("deepseek/deepseek-v4-pro", "DeepSeek V4 Pro", "Popular OpenRouter coding model"),
			openRouterNativeModel("tencent/hy3-preview", "Hy3 preview", "Popular OpenRouter coding model"),
			openRouterNativeModel("stepfun/step-3.7-flash", "Step 3.7 Flash", "Popular OpenRouter coding model"),
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

func completeModelMetadata(model Model) Model {
	known, ok := knownModels[strings.ToLower(model.Value)]
	if !ok {
		return model
	}
	if len(model.InputModalities) == 0 {
		model.InputModalities = cloneStrings(known.InputModalities)
	}
	return model
}

func cloneModel(model Model) Model {
	model.InputModalities = cloneStrings(model.InputModalities)
	model.Reasoning.Efforts = cloneStrings(model.Reasoning.Efforts)
	if model.Pricing != nil {
		pricing := *model.Pricing
		model.Pricing = &pricing
	}
	return model
}

func modelWithoutProviderReasoning(value, label, description string) Model {
	return Model{Value: value, Label: label, Description: description, Reasoning: Reasoning{Status: ReasoningUnavailable}}
}

func openRouterBackedModel(value, label, description, openRouterID string) Model {
	model := modelWithoutProviderReasoning(value, label, description)
	model.OpenRouterID = openRouterID
	model.Reasoning.Status = ReasoningPending
	return model
}

func openRouterNativeModel(value, label, description string) Model {
	return openRouterBackedModel(value, label, description, value)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
