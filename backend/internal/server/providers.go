package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/providerstore"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/storage"
)

// providerView is the API shape for a custom provider: the stored record plus its
// live key-configured status. The key value itself is never returned.
type providerView struct {
	providerstore.CustomProvider
	Configured       bool   `json:"configured"`
	ConnectionStatus string `json:"connection_status"`
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
	configured := s.modelProviderConfiguredStatus(record.ID, cfg, meta, true)
	return providerView{
		CustomProvider:   record,
		Configured:       configured,
		ConnectionStatus: modelProviderConnectionStatus(configured),
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
