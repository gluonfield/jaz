package modelcatalog

import (
	"context"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

type openAICompatibleModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
	} `json:"data"`
}

// FetchModels loads an OpenAI-style /models list from endpoint, with optional
// auth and extra headers.
func FetchModels(ctx context.Context, endpoint, apiKey string, headers map[string]string) ([]Model, error) {
	return fetchOpenAICompatibleModels(ctx, endpoint, apiKey, headers)
}

// fetchOpenAICompatibleModels loads {endpoint}, an OpenAI-style /models list.
func fetchOpenAICompatibleModels(ctx context.Context, endpoint, apiKey string, headers map[string]string) ([]Model, error) {
	var body openAICompatibleModelsResponse
	if err := fetchJSON(ctx, http.MethodGet, endpoint, apiKey, headers, nil, &body); err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(body.Data))
	for _, entry := range body.Data {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(entry.Name)
		if label == "" {
			label = id
		}
		model := modelWithoutProviderReasoning(id, label, "")
		model.ContextLength = entry.ContextLength
		out = append(out, completeModelMetadata(model))
	}
	return out, nil
}

// catalogEndpoint resolves where a provider's model list lives: an explicit
// models URL when the provider overrides it, else base URL + "/models".
func catalogEndpoint(metaBaseURL, modelsURL, cfgBaseURL string) string {
	if u := strings.TrimSpace(modelsURL); u != "" {
		return u
	}
	base := firstNonEmpty(cfgBaseURL, metaBaseURL)
	endpoint, err := provider.ModelsURL(base)
	if err != nil {
		return ""
	}
	return endpoint
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
