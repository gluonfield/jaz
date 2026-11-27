package cloud

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

type MCPHandler struct {
	service  *cloudsync.Service
	sessions sessionLoader
}

type sessionLoader interface {
	LoadSession(string) (storage.Session, error)
}

func NewMCPHandler(service *cloudsync.Service, sessions sessionLoader) *MCPHandler {
	return &MCPHandler{service: service, sessions: sessions}
}

func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session, err := h.sessions.LoadSession(r.Header.Get(mcpsession.HeaderName))
	if err != nil || storage.NormalizeWorkspaceID(session.WorkspaceID) == storage.LocalWorkspaceID {
		httpapi.WriteError(w, http.StatusForbidden, errors.New("a native Cloud-workspace session is required"))
		return
	}
	var body json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.Method == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid MCP request"))
		return
	}
	if len(request.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if request.Method == "ping" {
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": struct{}{}})
		return
	}
	raw, status, err := h.service.CallMCP(r.Context(), session, body, request.Method == "tools/call")
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}
