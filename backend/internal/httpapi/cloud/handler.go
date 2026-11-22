package cloud

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

const maxCloudJSONBodyBytes int64 = 4 << 20

type Handler struct {
	service *cloudsync.Service
}

func NewHandler(service *cloudsync.Service) *Handler {
	return &Handler{service: service}
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
		writeCloudError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Disconnect(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, h.service.Status(r.Context()))
}

func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.SyncNow(r.Context())
	if err != nil {
		writeCloudError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	raw, err := h.service.ListModels(r.Context())
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) ListMachines(w http.ResponseWriter, r *http.Request) {
	raw, err := h.service.ListMachines(r.Context())
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := h.service.ListSpaces(r.Context())
	if err != nil {
		writeCloudError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"spaces": spaces})
}

func (h *Handler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("space name is required"))
		return
	}
	created, err := h.service.CreateSpace(r.Context(), name)
	if err != nil {
		writeCloudError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) AddSpaceMember(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.AddSpaceMember(r.Context(), r.PathValue("id"), body)
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, status, raw)
}

func (h *Handler) GetInstruction(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.GetInstruction(
		r.Context(),
		r.PathValue("scope"),
		r.PathValue("scope_id"),
		r.PathValue("kind"),
	)
	if err != nil {
		writeCloudError(w, err)
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
		writeCloudError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, saved)
}

func (h *Handler) UsageSummary(w http.ResponseWriter, r *http.Request) {
	raw, err := h.service.GetUsageSummary(r.Context())
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) CreateTurn(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.CreateTurn(r.Context(), r.PathValue("id"), body)
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, status, raw)
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
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, http.StatusOK, raw)
}

func (h *Handler) PresignUpload(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.PresignUpload(r.Context(), r.PathValue("run_id"), body)
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, status, raw)
}

func (h *Handler) CommitUpload(w http.ResponseWriter, r *http.Request) {
	body, err := readLimitedBody(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	raw, status, err := h.service.CommitUpload(r.Context(), r.PathValue("run_id"), body)
	if err != nil {
		writeCloudError(w, err)
		return
	}
	writeRawJSON(w, status, raw)
}

func (h *Handler) ProxyRunFile(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.OpenRunFile(r.Context(), r.PathValue("run_id"), r.PathValue("path"))
	if err != nil {
		writeCloudError(w, err)
		return
	}
	defer resp.Body.Close()
	for _, header := range []string{"Content-Type", "Content-Disposition", "Content-Length", "ETag", "Cache-Control"} {
		if v := resp.Header.Get(header); v != "" {
			w.Header().Set(header, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func readLimitedBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCloudJSONBodyBytes))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func writeRawJSON(w http.ResponseWriter, status int, raw json.RawMessage) {
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

func writeCloudError(w http.ResponseWriter, err error) {
	var uerr *cloudsync.UpstreamError
	if errors.As(err, &uerr) {
		if len(uerr.Body) > 0 {
			writeRawJSON(w, uerr.StatusCode, uerr.Body)
			return
		}
		httpapi.WriteError(w, uerr.StatusCode, uerr)
		return
	}
	switch {
	case errors.Is(err, cloudsync.ErrInvalidURL):
		httpapi.WriteError(w, http.StatusBadRequest, err)
	case errors.Is(err, cloudsync.ErrNotConnected):
		httpapi.WriteError(w, http.StatusUnauthorized, err)
	default:
		httpapi.WriteError(w, http.StatusBadGateway, err)
	}
}
