package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
)

const (
	modelProviderStatusConnected    = "connected"
	modelProviderStatusNotConnected = "not_connected"
)

var modelProviderStatusHTTPClient = &http.Client{Timeout: 900 * time.Millisecond}

type providerStatusResponse struct {
	ConnectionStatus string `json:"connection_status"`
}

type modelProviderStatusResponse struct {
	Providers []modelProviderConnectionStatus `json:"providers"`
}

type modelProviderConnectionStatus struct {
	ID               string `json:"id"`
	ConnectionStatus string `json:"connection_status"`
}

func (s *Server) handleProviderStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("provider id is required"))
		return
	}
	input, ok := s.resolvedModelProvider(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("provider %q not found", id))
		return
	}
	writeJSON(w, http.StatusOK, providerStatusResponse{
		ConnectionStatus: s.modelProviderConnectionStatus(r.Context(), input),
	})
}

func (s *Server) handleProviderStatuses(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, modelProviderStatusResponse{
		Providers: s.modelProviderConnectionStatuses(r.Context()),
	})
}

func (s *Server) modelProviderConnectionStatuses(ctx context.Context) []modelProviderConnectionStatus {
	inputs := s.resolvedModelProviders()
	out := make([]modelProviderConnectionStatus, len(inputs))
	var wg sync.WaitGroup
	for i, input := range inputs {
		i, input := i, input
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = modelProviderConnectionStatus{
				ID:               input.ID,
				ConnectionStatus: s.modelProviderConnectionStatus(ctx, input),
			}
		}()
	}
	wg.Wait()
	return out
}

func (s *Server) modelProviderConnectionStatus(ctx context.Context, input resolvedModelProvider) string {
	if !s.modelProviderConfigReady(input.ID, input.Config, input.Meta) {
		return modelProviderStatusNotConnected
	}
	if input.Meta.RequiresAPIKey {
		return modelProviderStatusConnected
	}
	if input.Meta.OpenAICompatible || strings.EqualFold(strings.TrimSpace(input.Config.Type), "openai-compatible") {
		if probeOpenAICompatibleProvider(ctx, modelProviderStatusHTTPClient, input.Meta.BaseURL) != nil {
			return modelProviderStatusNotConnected
		}
	}
	return modelProviderStatusConnected
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func probeOpenAICompatibleProvider(ctx context.Context, client httpDoer, baseURL string) error {
	endpoint, err := provider.ModelsURL(baseURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
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
