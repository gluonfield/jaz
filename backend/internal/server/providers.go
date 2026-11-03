package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/providerstore"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/storage"
)

// providerView is the API shape for a custom provider: the stored record plus
// config readiness. The key value itself is never returned.
type providerView struct {
	providerstore.CustomProvider
	Configured bool `json:"configured"`
}

// providerRequest is the create/update body — the editable fields plus an
// optional write-only API key.
type providerRequest struct {
	providerstore.Input
	APIKey string `json:"api_key,omitempty"`
}

func (s *Server) settingsStore() (storage.SettingsStorage, bool) {
	store, ok := s.Store.(storage.SettingsStorage)
	return store, ok
}

// customProviderIDSet returns the set of DB-backed custom provider ids so the
// settings response can mark which providers are user-editable.
func (s *Server) customProviderIDSet() map[string]bool {
	store, ok := s.settingsStore()
	if !ok {
		return nil
	}
	records, err := providerstore.List(store)
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(records))
	for _, record := range records {
		set[record.ID] = true
	}
	return set
}

func (s *Server) providerView(record providerstore.CustomProvider) providerView {
	cfg := record.Config()
	meta := provider.ApplyModelProviderConfig(provider.ModelProvider{ID: record.ID}, cfg)
	return providerView{
		CustomProvider: record,
		Configured:     s.modelProviderConfigReady(record.ID, cfg, meta),
	}
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	store, ok := s.settingsStore()
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	records, err := providerstore.List(store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]providerView, 0, len(records))
	for _, record := range records {
		out = append(out, s.providerView(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	store, ok := s.settingsStore()
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	var input providerRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	record, err := providerstore.Create(store, input.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if status, err := s.applyProviderRecordKey(r, record, input.APIKey); err != nil {
		// Don't leave a half-created provider behind if the key write is rejected.
		_, _ = providerstore.Delete(store, record.ID)
		_ = s.reloadProviders()
		writeError(w, status, err)
		return
	}
	if err := s.reloadProviders(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.providerView(record))
}

func (s *Server) handleProviderAction(w http.ResponseWriter, r *http.Request) {
	store, ok := s.settingsStore()
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/providers/"))
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, fmt.Errorf("provider id is required"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input providerRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		record, err := providerstore.Update(store, id, input.Input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if status, err := s.applyProviderRecordKey(r, record, input.APIKey); err != nil {
			writeError(w, status, err)
			return
		}
		if err := s.reloadProviders(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.providerView(record))
	case http.MethodDelete:
		record, err := providerstore.Delete(store, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if env := strings.TrimSpace(record.APIKeyEnv); env != "" {
			_ = runtimeenv.Remove(s.runtimeKeyEnvPath(), env)
		}
		if err := s.reloadProviders(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

// applyProviderRecordKey writes a provider's API key (when supplied) to the
// runtime .env under the host-only guard. The env var is the record's stable
// APIKeyEnv, so this needs no live source lookup. Returns the HTTP status to use
// on failure (0 on success).
func (s *Server) applyProviderRecordKey(r *http.Request, record providerstore.CustomProvider, apiKey string) (int, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return 0, nil
	}
	env := strings.TrimSpace(record.APIKeyEnv)
	if env == "" {
		return http.StatusBadRequest, fmt.Errorf("provider %q has no API key env var", record.ID)
	}
	if !s.providerKeySetupAllowed(r) {
		return http.StatusForbidden, fmt.Errorf("key setup is only available from the backend host")
	}
	if err := s.saveRuntimeKeyUpdates(map[string]string{env: apiKey}); err != nil {
		return http.StatusBadRequest, err
	}
	return 0, nil
}

// handleFetchProviderModels probes a provider's live model catalog so the
// settings form can fill its models list. Works for both stored providers
// (id: uses the stored key) and unsaved drafts (api_key in the body). The
// catalog URL is the models_url override when set, else base URL + "/models".
func (s *Server) handleFetchProviderModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var body struct {
		ID        string            `json:"id,omitempty"`
		BaseURL   string            `json:"base_url,omitempty"`
		ModelsURL string            `json:"models_url,omitempty"`
		Headers   map[string]string `json:"headers,omitempty"`
		APIKey    string            `json:"api_key,omitempty"`
		Save      bool              `json:"save,omitempty"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	headers := body.Headers
	modelsURL := strings.TrimSpace(body.ModelsURL)
	apiKey := strings.TrimSpace(body.APIKey)
	if id := strings.TrimSpace(body.ID); id != "" {
		store, ok := s.settingsStore()
		if !ok {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
			return
		}
		record, found, err := providerstore.Get(store, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if found {
			if baseURL == "" {
				baseURL = record.BaseURL
			}
			if modelsURL == "" {
				modelsURL = record.ModelsURL
			}
			if len(headers) == 0 {
				headers = record.Headers
			}
			if apiKey == "" && s.runtimeKeyEnvPath() != "" {
				if value, ok := runtimeenv.Lookup(s.runtimeKeyEnvPath(), record.APIKeyEnv); ok {
					apiKey = value
				}
			}
		}
	}
	if baseURL == "" && modelsURL == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("base_url or models_url is required"))
		return
	}
	// Some gateways serve the catalog on a sibling path (OpenCode Go: chat
	// under /openai/v1, models under /v1), so try the obvious candidates.
	endpoints := []string{}
	if modelsURL != "" {
		endpoints = append(endpoints, modelsURL)
	}
	if u, err := provider.ModelsURL(baseURL); err == nil {
		endpoints = append(endpoints, u)
	}
	if trimmed := strings.TrimSuffix(baseURL, "/"); strings.HasSuffix(trimmed, "/openai/v1") {
		endpoints = append(endpoints, strings.TrimSuffix(trimmed, "/openai/v1")+"/v1/models")
	}
	var (
		models  []modelcatalog.Model
		working string
		lastErr error
	)
	for _, endpoint := range endpoints {
		attempt, err := modelcatalog.FetchModels(r.Context(), endpoint, apiKey, headers)
		if err == nil && len(attempt) > 0 {
			models, working, lastErr = attempt, endpoint, nil
			break
		}
		lastErr = err
	}
	if lastErr != nil || len(models) == 0 {
		status := http.StatusBadGateway
		if lastErr == nil {
			lastErr = fmt.Errorf("no models returned")
		}
		writeError(w, status, lastErr)
		return
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.Value)
	}
	saved := false
	if providerID := strings.TrimSpace(body.ID); body.Save && providerID != "" {
		store, ok := s.settingsStore()
		if ok {
			if record, found, err := providerstore.Get(store, providerID); err == nil && found {
				input := providerstore.Input{
					Label:        record.Label,
					BaseURL:      record.BaseURL,
					DefaultModel: record.DefaultModel,
					Capabilities: record.Capabilities,
					Headers:      record.Headers,
					Models:       ids,
					ModelsURL:    firstNonEmptyString(working, record.ModelsURL),
					Icon:         record.Icon,
				}
				if _, err := providerstore.Update(store, providerID, input); err == nil {
					saved = true
					if err := s.reloadProviders(); err == nil {
						_ = err // provider reload is best-effort for the response
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": ids, "catalog_url": working, "saved": saved})
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
