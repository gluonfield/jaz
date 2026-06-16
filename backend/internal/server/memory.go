package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
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
	DreamAgent       string              `json:"dream_agent,omitempty"`
	SchedulerRunning bool                `json:"scheduler_running"`
	Root             string              `json:"root"`
	DBPath           string              `json:"db_path"`
	Doctor           jazmem.DoctorReport `json:"doctor"`
	Horizons         []memoryHorizon     `json:"horizons"`
	Tasks            []jazmem.TaskStatus `json:"tasks"`
	MCPURL           string              `json:"mcp_url,omitempty"`
}

type memorySettingsInput struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	DreamAgent *string `json:"dream_agent,omitempty"`
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
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		return memoryStatusResponse{}, fmt.Errorf("settings store is not configured")
	}
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
	settings, err := jazsettings.LoadMemorySettings(store)
	if err != nil {
		return memoryStatusResponse{}, err
	}
	return memoryStatusResponse{
		Enabled:          s.Memory.Enabled(),
		DreamAgent:       settings.DreamAgent,
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
	var input memorySettingsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := s.normalizeMemorySettingsInput(store, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := jazsettings.SaveMemorySettings(store, settings); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.Memory.Scheduler != nil {
		if settings.Enabled {
			s.Memory.Scheduler.Start()
		} else {
			s.Memory.Scheduler.Stop()
		}
	}
	s.JazTools.Sync()
	s.refreshMCP()
	status, err := s.memoryStatus(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) normalizeMemorySettingsInput(store storage.SettingsStorage, input memorySettingsInput) (jazsettings.MemorySettings, error) {
	settings, err := jazsettings.LoadMemorySettings(store)
	if err != nil {
		return jazsettings.MemorySettings{}, err
	}
	if input.Enabled != nil {
		settings.Enabled = *input.Enabled
	}
	if input.DreamAgent != nil {
		settings.DreamAgent = acp.CanonicalAgentName(*input.DreamAgent)
	}
	if settings.DreamAgent == "" {
		return settings, nil
	}
	agentSettings, err := s.loadAgentSettings(store)
	if err != nil {
		return jazsettings.MemorySettings{}, err
	}
	if current, ok := agentSettings.ACP[settings.DreamAgent]; !ok {
		return jazsettings.MemorySettings{}, fmt.Errorf("unknown dream agent %q", settings.DreamAgent)
	} else if !current.Enabled {
		return jazsettings.MemorySettings{}, fmt.Errorf("dream agent %q is not enabled", settings.DreamAgent)
	}
	return settings, nil
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
	report, err := s.Memory.RunIndexTask(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleMemoryDream(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemory(w) {
		return
	}
	report, err := s.Memory.RunDreamTask(r.Context(), jazmem.DreamOptions{})
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

func (s *Server) jazToolsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.JazTools.Handler().ServeHTTP(w, r)
	})
}

func (s *Server) memoryAPIHandler() http.Handler {
	return s.memoryGated(func() http.Handler { return s.Memory.APIHandler() })
}
