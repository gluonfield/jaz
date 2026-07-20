package modelcatalog

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const ollamaShowConcurrency = 8

type ollamaShowResponse struct {
	Capabilities []string `json:"capabilities"`
}

type ollamaShowRequest struct {
	Model string `json:"model"`
}

func fetchOllamaModels(ctx context.Context, baseURL string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, modelCatalogRequestTimeout)
	defer cancel()
	models, err := fetchOpenAICompatibleModels(ctx, baseURL, "")
	if err != nil {
		return nil, err
	}
	endpoint, err := ollamaShowURL(baseURL)
	if err != nil {
		return nil, err
	}
	limit := make(chan struct{}, ollamaShowConcurrency)
	var probes sync.WaitGroup
	for i := range models {
		probes.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()
			var show ollamaShowResponse
			if err := fetchJSON(ctx, http.MethodPost, endpoint, "", ollamaShowRequest{Model: models[i].Value}, &show); err != nil {
				return
			}
			for _, capability := range show.Capabilities {
				if strings.EqualFold(strings.TrimSpace(capability), "thinking") {
					models[i].Reasoning.Status = ReasoningReady
					models[i].Reasoning.Automatic = true
					return
				}
			}
		})
	}
	probes.Wait()
	return models, nil
}

func ollamaShowURL(baseURL string) (string, error) {
	endpoint, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(endpoint.Path, "/")
	path = strings.TrimSuffix(path, "/v1")
	endpoint.Path = strings.TrimRight(path, "/") + "/api/show"
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint.String(), nil
}
