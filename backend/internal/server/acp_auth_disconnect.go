package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/storage"
)

// handleDisconnectACPAuth removes an agent's credential so it can be re-connected
// or switched. It deletes only what Jaz owns: the Jaz-managed API key env var,
// and an OAuth credential that lives in Jaz's own profile (or Grok's auth file).
// It never touches the user's global ~/.claude.json / ~/.codex config.
func (s *Server) handleDisconnectACPAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	agent := acp.CanonicalAgentName(r.PathValue("agent"))
	if _, ok := s.acpAgentCatalog().Agent(agent); !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown acp agent %q", agent))
		return
	}
	if !s.providerKeySetupAllowed(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("disconnecting is only available from the backend host"))
		return
	}
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	defaults, err := s.loadAgentSettings(store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg, _, err := s.acpProbeConfig(agent, defaults)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	auth := acp.ProbeAgentAuth(agent, cfg, s.runtimeRoot(), nil)

	if spec, ok := acp.AgentAPIKey(agent); ok && strings.TrimSpace(spec.SourceEnv) != "" {
		if err := runtimeenv.Remove(s.runtimeKeyEnvPath(), spec.SourceEnv); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	if auth.AuthKind == acp.AuthKindOAuth && strings.TrimSpace(auth.StoragePath) != "" {
		if pathUnderRoot(auth.StoragePath, s.runtimeRoot()) || agent == acp.AgentGrok {
			if err := removeACPCredentialFiles(agent, auth.StoragePath); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
	}

	fresh := acp.ProbeAgentAuth(agent, cfg, s.runtimeRoot(), nil)
	writeJSON(w, http.StatusOK, newACPAuthStatusResponse(fresh))
}

func removeACPCredentialFiles(agent, storagePath string) error {
	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Claude can keep a sibling .credentials.json alongside .claude.json.
	if acp.CanonicalAgentName(agent) == acp.AgentClaude {
		creds := filepath.Join(filepath.Dir(storagePath), ".credentials.json")
		if err := os.Remove(creds); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func pathUnderRoot(path, root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
