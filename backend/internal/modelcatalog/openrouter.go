package modelcatalog

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

var openRouterReasoningEfforts = map[string]struct{}{
	"none":    {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
	"max":     {},
}

type openRouterModelResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	ContextLength int                    `json:"context_length"`
	Architecture  openRouterArchitecture `json:"architecture"`
	Pricing       openRouterPricing      `json:"pricing"`
	Reasoning     *openRouterReasoning   `json:"reasoning"`
}

type openRouterArchitecture struct {
	InputModalities []string `json:"input_modalities"`
}

type openRouterPricing struct {
	Prompt          string `json:"prompt"`
	Completion      string `json:"completion"`
	InputCacheRead  string `json:"input_cache_read"`
	InputCacheWrite string `json:"input_cache_write"`
}

type openRouterReasoning struct {
	Mandatory        bool     `json:"mandatory"`
	SupportedEfforts []string `json:"supported_efforts"`
	DefaultEffort    string   `json:"default_effort"`
}

func fetchOpenRouterModels(ctx context.Context, baseURL string) ([]Model, error) {
	endpoint, err := openRouterModelsURL(baseURL)
	if err != nil {
		return nil, err
	}
	var body openRouterModelResponse
	if err := fetchJSON(ctx, http.MethodGet, endpoint, nil, &body); err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(body.Data))
	for _, model := range body.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		description := strings.TrimSpace(model.Description)
		if description == "" {
			description = id
		}
		out = append(out, Model{
			Value:           id,
			Label:           openRouterModelLabel(model.Name, id),
			Description:     description,
			ContextLength:   model.ContextLength,
			InputModalities: cleanInputModalities(model.Architecture.InputModalities),
			Pricing:         parseOpenRouterPricing(model.Pricing),
			Reasoning: Reasoning{
				Status:        ReasoningReady,
				Efforts:       cleanOpenRouterReasoningEfforts(model.Reasoning),
				DefaultEffort: openRouterDefaultEffort(model.Reasoning),
				Mandatory:     model.Reasoning != nil && model.Reasoning.Mandatory,
			},
		})
	}
	return out, nil
}

func cleanInputModalities(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if (value == "text" || value == "image") && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

func openRouterModelsURL(baseURL string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	endpoint, err := provider.ModelsURL(baseURL)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("output_modalities", "text,image")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func openRouterModelLabel(name, fallback string) string {
	label := strings.TrimSpace(name)
	if label == "" {
		return fallback
	}
	if _, rest, ok := strings.Cut(label, ": "); ok {
		return rest
	}
	return label
}

func parseOpenRouterPricing(raw openRouterPricing) *Pricing {
	input := perToken(raw.Prompt)
	output := perToken(raw.Completion)
	if input == 0 && output == 0 {
		return nil
	}
	cacheRead := perToken(raw.InputCacheRead)
	if cacheRead == 0 {
		cacheRead = input
	}
	cacheWrite := perToken(raw.InputCacheWrite)
	if cacheWrite == 0 {
		cacheWrite = input
	}
	return &Pricing{
		Input:      input,
		Output:     output,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
	}
}

func perToken(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func cleanOpenRouterReasoningEfforts(reasoning *openRouterReasoning) []string {
	if reasoning == nil {
		return []string{}
	}
	out := []string{}
	for _, effort := range reasoning.SupportedEfforts {
		effort = strings.ToLower(strings.TrimSpace(effort))
		if _, ok := openRouterReasoningEfforts[effort]; ok {
			out = append(out, effort)
		}
	}
	sortReasoningEfforts(out)
	return out
}

func openRouterDefaultEffort(reasoning *openRouterReasoning) string {
	if reasoning == nil {
		return ""
	}
	return strings.TrimSpace(reasoning.DefaultEffort)
}
