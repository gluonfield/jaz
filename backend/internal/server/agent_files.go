package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/coordinator"
)

type agentFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

func allowedAgentFile(name string) bool {
	return slices.Contains(coordinator.PromptFiles, name)
}

func (s *Server) handleListAgentFiles(w http.ResponseWriter, r *http.Request) {
	if s.Root == "" {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("agent root is not configured"))
		return
	}
	files := make([]agentFile, 0, len(coordinator.PromptFiles))
	for _, name := range coordinator.PromptFiles {
		data, err := os.ReadFile(filepath.Join(s.Root, name))
		switch {
		case err == nil:
			files = append(files, agentFile{Name: name, Content: string(data), Exists: true})
		case os.IsNotExist(err):
			files = append(files, agentFile{Name: name})
		default:
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "root": s.Root})
}

func (s *Server) handleWriteAgentFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !allowedAgentFile(name) || name != filepath.Base(name) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown agent file %q", name))
		return
	}
	if s.Root == "" {
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
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(filepath.Join(s.Root, name), []byte(req.Content), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, agentFile{Name: name, Content: req.Content, Exists: true})
}

// withCORS allows the desktop renderer (a loopback vite dev server or the
// packaged file:// origin, which sends "null") to call the API from another
// origin.
func allowedOrigin(origin string) bool {
	return origin == "null" ||
		strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:")
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); allowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
