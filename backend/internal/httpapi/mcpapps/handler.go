package mcpapps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/mcp"
)

// Runtime serves MCP Apps from the servers' live, authenticated sessions.
type Runtime interface {
	Entrypoints(ctx context.Context) ([]mcp.Entrypoint, error)
	ReadApp(ctx context.Context, serverID, tool string) (string, error)
	CallAppTool(ctx context.Context, serverID, name string, arguments json.RawMessage, meta mcpsdk.Meta) (*mcpsdk.CallToolResult, error)
}

type CloudProxy interface {
	ProxyActiveWorkspace(w http.ResponseWriter, r *http.Request) bool
}

type Handler struct {
	runtime Runtime
	cloud   CloudProxy
}

type listResponse struct {
	Entrypoints []mcp.Entrypoint `json:"entrypoints"`
}

type resourceResponse struct {
	HTML string `json:"html"`
}

type callToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      mcpsdk.Meta     `json:"_meta,omitempty"`
}

func NewHandler(runtime Runtime) *Handler {
	return &Handler{runtime: runtime}
}

func (h *Handler) WithCloud(cloud CloudProxy) *Handler {
	h.cloud = cloud
	return h
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.cloud != nil && h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	entrypoints, err := h.runtime.Entrypoints(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, listResponse{Entrypoints: entrypoints})
}

func (h *Handler) Resource(w http.ResponseWriter, r *http.Request) {
	if h.cloud != nil && h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	html, err := h.runtime.ReadApp(r.Context(), r.PathValue("server"), r.URL.Query().Get("tool"))
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resourceResponse{HTML: html})
}

func (h *Handler) CallTool(w http.ResponseWriter, r *http.Request) {
	if h.cloud != nil && h.cloud.ProxyActiveWorkspace(w, r) {
		return
	}
	var req callToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("tool name is required"))
		return
	}
	result, err := h.runtime.CallAppTool(r.Context(), r.PathValue("server"), req.Name, req.Arguments, req.Meta)
	if err != nil {
		httpapi.WriteError(w, statusFor(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, mcp.ErrAppNotFound):
		return http.StatusNotFound
	case errors.Is(err, mcp.ErrAppToolDenied):
		return http.StatusForbidden
	default:
		return http.StatusBadGateway
	}
}
