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

func (s *Server) modelProviderConfiguredStatus(id string, cfg provider.ModelProviderConfig, meta provider.ModelProvider, probeNoKey bool) bool {
	if meta.RequiresAPIKey {
		return s.modelProviderKeyConfigured(id, cfg, meta)
	}
	if meta.OpenAICompatible || strings.EqualFold(strings.TrimSpace(cfg.Type), "openai-compatible") {
		if !probeNoKey {
			return false
		}
		return probeOpenAICompatibleProvider(meta.BaseURL) == nil
	}
	return true
}

func modelProviderConnectionStatus(configured bool) string {
	if configured {
		return modelProviderStatusConnected
	}
	return modelProviderStatusNotConnected
}

func probeOpenAICompatibleProvider(baseURL string) error {
	endpoint, err := openAICompatibleModelsURL(baseURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
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
