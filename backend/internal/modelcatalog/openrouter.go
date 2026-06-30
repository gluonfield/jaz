package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	ContextLength int                  `json:"context_length"`
	Pricing       openRouterPricing    `json:"pricing"`
	Reasoning     *openRouterReasoning `json:"reasoning"`
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
	endpoint, err := providerModelsURL(baseURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("model provider models request failed: %d", res.StatusCode)
	}
	var body openRouterModelResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(body.Data))
	for _, model := range body.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		out = append(out, Model{
			Value:                  id,
			Label:                  openRouterModelLabel(model.Name, id),
			Description:            id,
			ContextLength:          model.ContextLength,
			Pricing:                parseOpenRouterPricing(model.Pricing),
			ReasoningEfforts:       cleanOpenRouterReasoningEfforts(model.Reasoning),
			ReasoningDefaultEffort: openRouterDefaultEffort(model.Reasoning),
			ReasoningMandatory:     model.Reasoning != nil && model.Reasoning.Mandatory,
		})
	}
	return out, nil
}

func providerModelsURL(baseURL string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = "https://openrouter.ai/api/v1"
	}
	u, err := url.Parse(raw + "/models")
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
	return out
}

func openRouterDefaultEffort(reasoning *openRouterReasoning) string {
	if reasoning == nil {
		return ""
	}
	return strings.TrimSpace(reasoning.DefaultEffort)
}
