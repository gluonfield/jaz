package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
)

const (
	modelProviderStatusConnected    = "connected"
	modelProviderStatusNotConnected = "not_connected"
)

type providerStatusResponse struct {
	ConnectionStatus string `json:"connection_status"`
}

func (s *Server) handleProviderStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("provider id is required"))
		return
	}
	cfg, meta, ok := s.modelProviderMeta(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("provider %q not found", id))
		return
	}
	writeJSON(w, http.StatusOK, providerStatusResponse{
		ConnectionStatus: modelProviderConnectionStatus(s.modelProviderConnectionOK(r.Context(), id, cfg, meta)),
	})
}

func (s *Server) modelProviderMeta(id string) (provider.ModelProviderConfig, provider.ModelProvider, bool) {
	cfg := s.modelProviders()[id]
	meta, ok := provider.ModelProviderByID(id)
	if !ok {
		if !provider.ModelProviderConfigPresent(cfg) {
			return cfg, provider.ModelProvider{}, false
		}
		meta = provider.ModelProvider{ID: id}
	}
	return cfg, provider.ApplyModelProviderConfig(meta, cfg), true
}

func (s *Server) modelProviderConnectionOK(ctx context.Context, id string, cfg provider.ModelProviderConfig, meta provider.ModelProvider) bool {
	if meta.RequiresAPIKey {
		return s.modelProviderKeyConfigured(id, cfg, meta)
	}
	if meta.OpenAICompatible || strings.EqualFold(strings.TrimSpace(cfg.Type), "openai-compatible") {
		return probeOpenAICompatibleProvider(ctx, meta.BaseURL) == nil
	}
	return true
}

func modelProviderConnectionStatus(ok bool) string {
	if ok {
		return modelProviderStatusConnected
	}
	return modelProviderStatusNotConnected
}

func probeOpenAICompatibleProvider(ctx context.Context, baseURL string) error {
	endpoint, err := openAICompatibleModelsURL(baseURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("models endpoint returned %d", resp.StatusCode)
	}
	return nil
}

func openAICompatibleModelsURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid provider url %q", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/models"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
