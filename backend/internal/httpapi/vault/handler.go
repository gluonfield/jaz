package vault

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
	vaultcore "github.com/wins/jaz/backend/internal/vault"
)

const (
	maxVaultBodyBytes int64 = 1 << 20
	maxCSVBodyBytes   int64 = 8 << 20
)

type CloudProxy interface {
	ProxyActiveWorkspace(w http.ResponseWriter, r *http.Request) bool
}

type Handler struct {
	service *vaultcore.Service
	cloud   CloudProxy
}

func NewHandler(service *vaultcore.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) WithCloud(cloud CloudProxy) *Handler {
	h.cloud = cloud
	return h
}

func (h *Handler) proxyCloud(w http.ResponseWriter, r *http.Request) bool {
	return h.cloud != nil && h.cloud.ProxyActiveWorkspace(w, r)
}

func writeVaultError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrVaultItemNotFound):
		httpapi.WriteError(w, http.StatusNotFound, err)
	case errors.Is(err, vaultcore.ErrInvalidInput):
		httpapi.WriteError(w, http.StatusBadRequest, err)
	default:
		httpapi.WriteError(w, http.StatusInternalServerError, err)
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	q := r.URL.Query()
	items, err := h.service.List(r.Context(), strings.TrimSpace(q.Get("kind")), strings.TrimSpace(q.Get("domain")))
	if err != nil {
		writeVaultError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	var body vaultcore.CreateInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxVaultBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	item, err := h.service.Create(r.Context(), body)
	if err != nil {
		writeVaultError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	var body vaultcore.UpdateInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxVaultBodyBytes)).Decode(&body); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	item, err := h.service.Update(r.Context(), strings.TrimSpace(r.PathValue("id")), body)
	if err != nil {
		writeVaultError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := h.service.Delete(r.Context(), id); err != nil {
		writeVaultError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (h *Handler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	if h.proxyCloud(w, r) {
		return
	}
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	var csvText string
	if strings.HasPrefix(ct, "application/json") {
		var body struct {
			CSV string `json:"csv"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCSVBodyBytes)).Decode(&body); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("invalid json"))
			return
		}
		csvText = body.CSV
	} else {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCSVBodyBytes))
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, errors.New("failed to read CSV payload"))
			return
		}
		csvText = string(raw)
	}
	result, err := h.service.ImportCSV(r.Context(), csvText)
	if err != nil {
		writeVaultError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}
