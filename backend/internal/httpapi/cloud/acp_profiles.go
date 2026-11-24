package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type ACPProfileHandler struct {
	service *cloudsync.Service
	runner  *acp.CloudLoginRunner
}

func NewACPProfileHandler(service *cloudsync.Service, runner *acp.CloudLoginRunner) *ACPProfileHandler {
	return &ACPProfileHandler{service: service, runner: runner}
}

func (h *ACPProfileHandler) Proxy(w http.ResponseWriter, r *http.Request) {
	if !h.service.ProxyActiveWorkspace(w, r) {
		WriteError(w, cloudsync.ErrNotConnected)
		return
	}
	if r.Method != http.MethodGet || strings.Contains(r.URL.Path, "/login/") {
		h.service.InvalidateModelsCache()
	}
}

func (h *ACPProfileHandler) StartDesktopLogin(w http.ResponseWriter, r *http.Request) {
	if h.runner == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, errors.New("desktop OAuth sign-in is unavailable"))
		return
	}
	workspaceID := h.service.ActiveWorkspaceID()
	if workspaceID == "" || workspaceID == cloudsync.LocalWorkspaceID {
		WriteError(w, cloudsync.ErrNotConnected)
		return
	}
	agent := strings.ToLower(strings.TrimSpace(r.PathValue("agent")))
	snap, err := h.runner.Start(agent, func(ctx context.Context, normAgent string, files map[string][]byte) error {
		encoded := make(map[string]string, len(files))
		for rel, raw := range files {
			encoded[rel] = base64.StdEncoding.EncodeToString(raw)
		}
		payload, err := json.Marshal(map[string]any{"files": encoded})
		if err != nil {
			return err
		}
		_, status, err := h.service.PutACPProfileForWorkspace(ctx, workspaceID, normAgent, payload)
		if err != nil {
			return err
		}
		if status >= 300 {
			return fmt.Errorf("cloud returned status %d saving %s profile", status, normAgent)
		}
		return nil
	})
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]any{"session": snap})
}

func (h *ACPProfileHandler) GetDesktopLogin(w http.ResponseWriter, r *http.Request) {
	if h.runner == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, errors.New("desktop OAuth sign-in is unavailable"))
		return
	}
	snap, err := h.runner.Get(r.PathValue("agent"), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, acp.ErrCloudLoginNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"session": snap})
}
