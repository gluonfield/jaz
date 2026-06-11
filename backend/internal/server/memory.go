package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type memoryHorizon struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Chars    int    `json:"chars"`
	MaxChars int    `json:"max_chars"`
}

type memoryStatusResponse struct {
	Enabled          bool                `json:"enabled"`
	SchedulerRunning bool                `json:"scheduler_running"`
	Root             string              `json:"root"`
	DBPath           string              `json:"db_path"`
	Doctor           jazmem.DoctorReport `json:"doctor"`
	Horizons         []memoryHorizon     `json:"horizons"`
	Tasks            []jazmem.TaskStatus `json:"tasks"`
	MCPURL           string              `json:"mcp_url,omitempty"`
}

func (s *Server) requireMemory(w http.ResponseWriter) bool {
	if s.Memory == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("memory is not configured"))
		return false
	}
	return true
}

func (s *Server) handleMemoryStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemory(w) {
		return
	}
	status, err := s.memoryStatus(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) memoryStatus(r *http.Request) (memoryStatusResponse, error) {
	doctor, err := s.Memory.Doctor(r.Context())
	if err != nil {
		return memoryStatusResponse{}, err
	}
	tasks, err := s.Memory.SchedulerStatus(r.Context())
	if err != nil {
		return memoryStatusResponse{}, err
	}
	horizons := make([]memoryHorizon, 0, 2)
	for _, h := range []struct {
		name     string
		maxChars int
	}{
		{jazmem.LongTermFile, jazmem.LongTermMaxChars},
		{jazmem.ShortTermFile, jazmem.ShortTermMaxChars},
	} {
		content, err := s.Memory.ReadHorizonFile(h.name)
		if err != nil {
			return memoryStatusResponse{}, err
		}
		horizons = append(horizons, memoryHorizon{Name: h.name, Content: content, Chars: len(content), MaxChars: h.maxChars})
	}
	return memoryStatusResponse{
		Enabled:          s.Memory.Enabled(),
		SchedulerRunning: s.Memory.Scheduler != nil && s.Memory.Scheduler.Running(),
		Root:             s.Memory.Root(),
		DBPath:           s.Memory.DBPath(),
		Doctor:           doctor,
		Horizons:         horizons,
		Tasks:            tasks,
		MCPURL:           s.Memory.MCPURL(),
	}, nil
}

func (s *Server) handleMemoryUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemory(w) {
		return
	}
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("settings store is not configured"))
		return
	}
	var input jazsettings.MemorySettings
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := jazsettings.SaveMemorySettings(store, input); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.Memory.Scheduler != nil {
		if input.Enabled {
			s.Memory.Scheduler.Start()
		} else {
			s.Memory.Scheduler.Stop()
		}
	}
	s.refreshMCP()
	status, err := s.memoryStatus(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleMemoryHorizon(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemory(w) {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	var input struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Memory.WriteHorizonFile(name, input.Content); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	content, err := s.Memory.ReadHorizonFile(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryHorizon{Name: name, Content: content, Chars: len(content)})
}

func (s *Server) handleMemoryReindex(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemory(w) {
		return
	}
	report, err := s.Memory.Reindex(r.Context(), jazmem.ReindexOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// memoryGated wraps an embedded memory surface (MCP, jazmem HTTP API) so
// disabling memory cuts external access mid-session.
func (s *Server) memoryGated(handler func() http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Memory == nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("memory is not configured"))
			return
		}
		if !s.Memory.Enabled() {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("memory is disabled in settings"))
			return
		}
		handler().ServeHTTP(w, r)
	})
}

func (s *Server) memoryMCPHandler() http.Handler {
	return s.memoryGated(func() http.Handler { return s.Memory.MCPHandler() })
}

func (s *Server) memoryAPIHandler() http.Handler {
	return s.memoryGated(func() http.Handler { return s.Memory.APIHandler() })
}
