package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	if err := fetchModelCatalog(ctx, endpoint, &body); err != nil {
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

func fetchModelCatalog(ctx context.Context, endpoint string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("model provider models request failed: %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(target)
}
