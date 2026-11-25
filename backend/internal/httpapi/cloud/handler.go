package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/machinebridge"
	"github.com/wins/jaz/backend/internal/storage"
)

const maxCloudJSONBodyBytes int64 = 4 << 20

type Handler struct {
	service       *cloudsync.Service
	machineBridge machinebridge.Controller
}

func NewHandler(service *cloudsync.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) WithMachineBridge(bridge machinebridge.Controller) *Handler {
	h.machineBridge = bridge
	return h
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, h.service.Status(r.Context()))
}

func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	var req cloudsync.ConnectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	status, err := h.service.Connect(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) Switch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	workspaceID := strings.TrimSpace(body.WorkspaceID)
	if workspaceID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("workspace_id is required"))
		return
	}
	status, err := h.service.SwitchWorkspace(r.Context(), workspaceID)
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := h.service.Disconnect(r.Context(), body.WorkspaceID); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.service.Status(r.Context()))
}

func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.SyncNow(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	raw, err := h.service.ListModels(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) ListMachines(w http.ResponseWriter, r *http.Request) {
	raw, err := h.service.ListMachines(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	raw, err := h.service.ListMembers(r.Context(), q, limit)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) CreateMember(w http.ResponseWriter, r *http.Request) {
	body, err := ReadLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.CreateMember(r.Context(), body)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, r.PathValue("id"), h.service.UpdateMember)
}

func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	raw, status, err := h.service.DeleteMember(r.Context(), r.PathValue("id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *Handler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := h.service.ListSpaces(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
}

func (h *Handler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	var body cloudsync.CreateSpaceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("space name is required"))
		return
	}
	created, err := h.service.CreateSpace(r.Context(), body)
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) PatchSpace(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, r.PathValue("id"), h.service.PatchSpace)
}

func (h *Handler) AddSpaceMember(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, r.PathValue("id"), h.service.AddSpaceMember)
}

func (h *Handler) GetInstruction(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.GetInstruction(
		r.Context(),
		r.PathValue("scope"),
		r.PathValue("scope_id"),
		r.PathValue("kind"),
	)
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, doc)
}

func (h *Handler) PutInstruction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BodyMD          string `json:"body_md"`
		ExpectedVersion string `json:"expected_version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := h.service.SaveInstruction(
		r.Context(),
		r.PathValue("scope"),
		r.PathValue("scope_id"),
		r.PathValue("kind"),
		body.BodyMD,
		body.ExpectedVersion,
	)
	if err != nil {
		WriteError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, saved)
}

func (h *Handler) UsageSummary(w http.ResponseWriter, r *http.Request) {
	raw, err := h.service.GetUsageSummary(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) CreateTurn(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, r.PathValue("id"), h.service.CreateTurn)
}

func (h *Handler) CancelThread(w http.ResponseWriter, r *http.Request) {
	raw, status, err := h.service.CancelThread(r.Context(), r.PathValue("id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *Handler) SteerThread(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.SteerThread(r.Context(), r.PathValue("id"), body.Text, body.Mode)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *Handler) UpdateThreadUserState(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pinned                 *bool `json:"pinned"`
		Archived               *bool `json:"archived"`
		PersonalSourcesEnabled *bool `json:"personal_sources_enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	_, raw, err := h.service.UpdateUserState(r.Context(), r.PathValue("id"), storage.CloudThreadUserStatePatch{
		Pinned:                 body.Pinned,
		Archived:               body.Archived,
		PersonalSourcesEnabled: body.PersonalSourcesEnabled,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) PresignUpload(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, r.PathValue("run_id"), h.service.PresignUpload)
}

func (h *Handler) CommitUpload(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, r.PathValue("run_id"), h.service.CommitUpload)
}

func (h *Handler) proxyJSON(
	w http.ResponseWriter,
	r *http.Request,
	id string,
	fn func(context.Context, string, json.RawMessage) (json.RawMessage, int, error),
) {
	body, err := ReadLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := fn(r.Context(), id, body)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteRawJSON(w, status, raw)
}

func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	if !h.service.ProxyActiveWorkspace(w, r) {
		WriteError(w, cloudsync.ErrNotConnected)
	}
}

func (h *Handler) ProxyRunFile(w http.ResponseWriter, r *http.Request) {
	relpath := r.PathValue("path")
	resp, err := h.service.OpenRunFile(r.Context(), r.PathValue("run_id"), relpath)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		WriteError(w, cloudsync.ErrNotConnected)
		return
	}
	for _, header := range []string{"Content-Type", "Content-Disposition", "Content-Length", "ETag"} {
		if v := resp.Header.Get(header); v != "" {
			w.Header().Set(header, v)
		}
	}
	if r.URL.Query().Get("download") == "1" {
		if name := path.Base(strings.TrimSuffix(relpath, "/")); name != "" && name != "." && name != "/" {
			if disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name}); disposition != "" {
				w.Header().Set("Content-Disposition", disposition)
			}
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func ReadLimitedBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func WriteRawJSON(w http.ResponseWriter, status int, raw json.RawMessage) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(raw) == 0 {
		_, _ = w.Write([]byte("{}"))
		return
	}
	_, _ = w.Write(raw)
}

func WriteError(w http.ResponseWriter, err error) {
	var uerr *cloudsync.UpstreamError
	if errors.As(err, &uerr) {
		if len(uerr.Body) > 0 && uerr.StatusCode != http.StatusUnauthorized {
			WriteRawJSON(w, uerr.StatusCode, uerr.Body)
			return
		}
		httpapi.WriteError(w, uerr.StatusCode, uerr)
		return
	}
	switch {
	case errors.Is(err, cloudsync.ErrInvalidURL), errors.Is(err, cloudsync.ErrInvalidPath):
		httpapi.WriteError(w, http.StatusBadRequest, err)
	case errors.Is(err, cloudsync.ErrNotFound):
		httpapi.WriteError(w, http.StatusNotFound, err)
	case errors.Is(err, cloudsync.ErrNotConnected):
		httpapi.WriteError(w, http.StatusUnauthorized, err)
	default:
		httpapi.WriteError(w, http.StatusBadGateway, err)
	}
}
