package vault

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
	vaultcore "github.com/wins/jaz/backend/internal/vault"
)

const maxVaultBodyBytes int64 = 1 << 20

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
