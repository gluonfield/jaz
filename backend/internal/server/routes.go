package server

import (
	"net/http"
	"strings"
)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/auth/check", s.handleAuthCheck)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("GET /v1/sessions/", s.handleGetSession)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /v1/sessions/", s.handleSessionAction)
	mux.HandleFunc("GET /v1/loops", s.handleListLoops)
	mux.HandleFunc("POST /v1/loops", s.handleCreateLoop)
	mux.HandleFunc("/v1/loops/", s.handleLoopAction)
	mux.HandleFunc("GET /v1/boards", s.handleListBoards)
	mux.HandleFunc("POST /v1/boards", s.handleCreateBoard)
	mux.HandleFunc("/v1/boards/", s.handleBoardAction)
	mux.HandleFunc("GET /v1/widgets", s.handleListWidgets)
	mux.HandleFunc("GET /v1/widgets/assets/tailwind.js", s.handleWidgetTailwindAsset)
	mux.HandleFunc("/v1/widgets/", s.handleWidgetAction)
	mux.HandleFunc("GET /v1/music/chart-feed", s.handleMusicChartFeed)
	mux.HandleFunc("/v1/onboarding", s.handleOnboarding)
	mux.HandleFunc("/v1/settings/agents", s.handleAgentSettings)
	mux.HandleFunc("GET /v1/acp/agents", s.handleListACPAgents)
	mux.HandleFunc("GET /v1/projects", s.handleListProjects)
	mux.HandleFunc("POST /v1/projects", s.handleCreateProject)
	mux.HandleFunc("PUT /v1/projects/order", s.handleReorderProjects)
	mux.HandleFunc("GET /v1/filesystem/dirs", s.handleListFilesystemDirs)
	mux.HandleFunc("GET /v1/workspace/files", s.handleListWorkspaceFiles)
	mux.HandleFunc("GET /v1/skills", s.handleListSkills)
	mux.HandleFunc("GET /v1/mcp/servers", s.handleListMCPServers)
	mux.HandleFunc("POST /v1/mcp/servers", s.handleCreateMCPServer)
	mux.HandleFunc("PUT /v1/mcp/servers/", s.handleMCPServerAction)
	mux.HandleFunc("DELETE /v1/mcp/servers/", s.handleMCPServerAction)
	mux.HandleFunc("POST /v1/mcp/servers/", s.handleMCPServerAction)
	mux.HandleFunc("GET /v1/agent/files", s.handleListAgentFiles)
	mux.HandleFunc("PUT /v1/agent/files/{name}", s.handleWriteAgentFile)
	mux.HandleFunc("POST /v1/audio/transcribe", s.handleTranscribe)
	mux.HandleFunc("POST /v1/audio/speak", s.handleSpeak)
	mux.HandleFunc("GET /v1/memory", s.handleMemoryStatus)
	mux.HandleFunc("PUT /v1/memory", s.handleMemoryUpdate)
	mux.HandleFunc("PUT /v1/memory/horizons/{name}", s.handleMemoryHorizon)
	mux.HandleFunc("POST /v1/memory/reindex", s.handleMemoryReindex)
	mux.Handle("/mcp/jazmem", s.memoryMCPHandler())
	mux.Handle("/jazmem/", http.StripPrefix("/jazmem", s.memoryAPIHandler()))
	return withCORS(withGzip(s.withAuth(mux)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		OK:           true,
		AuthRequired: strings.TrimSpace(s.AuthKey) != "",
		Capabilities: healthCapabilities{
			SessionFileRead: true,
		},
	})
}

type healthResponse struct {
	OK           bool               `json:"ok"`
	AuthRequired bool               `json:"auth_required"`
	Capabilities healthCapabilities `json:"capabilities"`
}

type healthCapabilities struct {
	SessionFileRead bool `json:"session_file_read"`
}
