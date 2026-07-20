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

func fetchOpenAICompatibleModels(ctx context.Context, baseURL string) ([]Model, error) {
	endpoint, err := provider.ModelsURL(baseURL)
	if err != nil {
		return nil, err
	}
	var body openAICompatibleModelsResponse
	if err := fetchJSON(ctx, http.MethodGet, endpoint, nil, &body); err != nil {
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
		out = append(out, modelWithoutProviderReasoning(id, label, "", entry.ContextLength))
	}
	return out, nil
}
