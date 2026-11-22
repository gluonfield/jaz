package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/instructions"
)

type agentFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

func instructionKindForAgentFile(name string) (instructions.Kind, bool) {
	switch name {
	case "AGENTS.md":
		return instructions.KindAgents, true
	case "SOUL.md":
		return instructions.KindSoul, true
	case "BOTS.md":
		return instructions.KindBots, true
	default:
		return "", false
	}
}

func (s *Server) instructionsStore() instructions.Store {
	if s.Instructions != nil {
		return s.Instructions
	}
	return instructions.NewLocalFSStore(s.Root)
}

func (s *Server) handleListAgentFiles(w http.ResponseWriter, r *http.Request) {
	if s.Instructions == nil && s.Root == "" {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("agent root is not configured"))
		return
	}
	store := s.instructionsStore()
	files := make([]agentFile, 0, len(coordinator.PromptFiles))
	for _, name := range coordinator.PromptFiles {
		kind, ok := instructionKindForAgentFile(name)
		if !ok {
			continue
		}
		doc, err := store.Get(r.Context(), instructions.Key{Scope: instructions.ScopeUser, Kind: kind})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		files = append(files, agentFile{Name: name, Content: doc.Content, Exists: doc.Version != ""})
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
	if s.Instructions == nil && s.Root == "" {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("agent root is not configured"))
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	store := s.instructionsStore()
	key := instructions.Key{Scope: instructions.ScopeUser, Kind: kind}
	current, err := store.Get(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	saved, err := store.Save(r.Context(), key, req.Content, current.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, agentFile{Name: name, Content: saved.Content, Exists: true})
}
