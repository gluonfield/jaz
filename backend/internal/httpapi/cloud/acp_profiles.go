package cloud

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type LocalACPExporter interface {
	ExportLocalAuth(agent string) (acp.LocalAuthExport, error)
	ClearOwnedLocalAuth(agent, storagePath string) error
}

type ACPProfileHandler struct {
	service  *cloudsync.Service
	exporter LocalACPExporter
}

func NewACPProfileHandler(service *cloudsync.Service, exporter LocalACPExporter) *ACPProfileHandler {
	return &ACPProfileHandler{service: service, exporter: exporter}
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

func (h *ACPProfileHandler) SyncLocal(w http.ResponseWriter, r *http.Request) {
	if h.exporter == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, errors.New("local ACP auth export is unavailable"))
		return
	}
	agent := strings.ToLower(strings.TrimSpace(r.PathValue("agent")))
	if agent == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("agent is required"))
		return
	}
	var body struct {
		ClearLocal   bool   `json:"clear_local"`
		AccountLabel string `json:"account_label"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
	}
	exported, err := h.exporter.ExportLocalAuth(agent)
	if err != nil {
		switch {
		case errors.Is(err, acp.ErrUnsupportedCloudSyncAgent):
			httpapi.WriteError(w, http.StatusBadRequest, err)
		case errors.Is(err, acp.ErrLocalAuthNotFound):
			httpapi.WriteError(w, http.StatusNotFound, err)
		default:
			httpapi.WriteError(w, http.StatusInternalServerError, err)
		}
		return
	}
	encoded := make(map[string]string, len(exported.Files))
	for rel, raw := range exported.Files {
		encoded[rel] = base64.StdEncoding.EncodeToString(raw)
	}
	payload, err := json.Marshal(map[string]any{
		"files":         encoded,
		"account_label": strings.TrimSpace(body.AccountLabel),
	})
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	raw, status, err := h.service.PutACPProfile(r.Context(), exported.Agent, payload)
	if err != nil {
		WriteError(w, err)
		return
	}
	clearedLocal := false
	if body.ClearLocal && status < 300 && exported.OwnedByJaz {
		if clearErr := h.exporter.ClearOwnedLocalAuth(exported.Agent, exported.StoragePath); clearErr == nil {
			clearedLocal = true
		}
	}
	if status < 300 && len(raw) > 0 {
		var resp map[string]any
		if json.Unmarshal(raw, &resp) == nil {
			resp["cleared_local"] = clearedLocal
			resp["owned_by_jaz"] = exported.OwnedByJaz
			httpapi.WriteJSON(w, status, resp)
			return
		}
	}
	WriteRawJSON(w, status, raw)
}
