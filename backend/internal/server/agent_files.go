package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/instructions"
)

type agentFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
	Version string `json:"version,omitempty"`
}

func instructionKindForAgentFile(name string) (instructions.Kind, bool) {
	switch name {
	case "AGENTS.md":
		return instructions.KindAgents, true
	case "BOTS.md":
		return instructions.KindBots, true
	default:
		return "", false
	}
}

func (s *Server) handleListAgentFiles(w http.ResponseWriter, r *http.Request) {
	if s.Instructions == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("instructions store is not configured"))
		return
	}
	files := make([]agentFile, 0, len(coordinator.PromptFiles))
	for _, name := range coordinator.PromptFiles {
		kind, ok := instructionKindForAgentFile(name)
		if !ok {
			continue
		}
		doc, err := s.Instructions.Get(r.Context(), instructions.Key{Scope: instructions.ScopeUser, Kind: kind})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		files = append(files, agentFile{Name: name, Content: doc.Content, Exists: doc.Version != "", Version: doc.Version})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "root": s.Root})
}

func (s *Server) handleWriteAgentFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	kind, ok := instructionKindForAgentFile(name)
	if !ok || name != filepath.Base(name) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown agent file %q", name))
		return
	}
	if s.Instructions == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("instructions store is not configured"))
		return
	}
	var req struct {
		Content string  `json:"content"`
		Version *string `json:"version,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	key := instructions.Key{Scope: instructions.ScopeUser, Kind: kind}
	expectedVersion, hasExpected := expectedAgentFileVersion(r.Header.Get("If-Match"), req.Version)
	if !hasExpected {
		current, err := s.Instructions.Get(r.Context(), key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		expectedVersion = current.Version
	}
	saved, err := s.Instructions.Save(r.Context(), key, req.Content, expectedVersion)
	if errors.Is(err, instructions.ErrConflict) {
		if saved.Version != "" {
			w.Header().Set("ETag", `"`+saved.Version+`"`)
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": err.Error(),
			"file":  agentFile{Name: name, Content: saved.Content, Exists: saved.Version != "", Version: saved.Version},
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if saved.Version != "" {
		w.Header().Set("ETag", `"`+saved.Version+`"`)
	}
	writeJSON(w, http.StatusOK, agentFile{Name: name, Content: saved.Content, Exists: true, Version: saved.Version})
}

func expectedAgentFileVersion(ifMatch string, bodyVersion *string) (string, bool) {
	if raw := strings.TrimSpace(ifMatch); raw != "" {
		raw = strings.TrimPrefix(raw, "W/")
		return strings.Trim(raw, `"`), true
	}
	if bodyVersion != nil {
		return strings.TrimSpace(*bodyVersion), true
	}
	return "", false
}
