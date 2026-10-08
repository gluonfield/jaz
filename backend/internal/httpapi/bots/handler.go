package bots

import (
	"encoding/json"
	"errors"
	"net/http"

	botcore "github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

type Handler struct {
	bots        *botcore.Service
	attachments Attachments
}

// Attachments finds the files uploaded to a thread by their ids.
type Attachments interface {
	ResolveAttachments(sessionID string, ids []string) ([]storage.Attachment, error)
}

func NewHandler(bots *botcore.Service, attachments Attachments) *Handler {
	return &Handler{bots: bots, attachments: attachments}
}

type listResponse struct {
	Bots []botcore.Bot `json:"bots"`
}

type groupRequest struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type pinsRequest struct {
	IDs []string `json:"ids"`
}

type messageRequest struct {
	Text          string   `json:"text"`
	AttachmentIDs []string `json:"attachment_ids"`
}

func (h *Handler) List(w http.ResponseWriter, _ *http.Request) {
	bots, err := h.bots.List()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, listResponse{Bots: bots})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	bot, err := h.bots.Load(r.PathValue("bot"))
	writeBot(w, bot, err)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input botcore.CreateBot
	if !decode(w, r, &input) {
		return
	}
	bot, err := h.bots.Create(r.Context(), input)
	writeBot(w, bot, err)
}

func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var input groupRequest
	if !decode(w, r, &input) {
		return
	}
	bot, err := h.bots.CreateGroup(input.Name, input.Members)
	writeBot(w, bot, err)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var input botcore.UpdateBot
	if !decode(w, r, &input) {
		return
	}
	bot, err := h.bots.Update(r.Context(), r.PathValue("bot"), input)
	writeBot(w, bot, err)
}

func (h *Handler) Pin(w http.ResponseWriter, r *http.Request) {
	var input pinsRequest
	if !decode(w, r, &input) {
		return
	}
	if err := h.bots.Pin(input.IDs); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.bots.Delete(r.PathValue("bot")); err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	var input messageRequest
	if !decode(w, r, &input) {
		return
	}
	group := r.PathValue("bot")
	attachments, err := h.attachments.ResolveAttachments(group, input.AttachmentIDs)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.bots.Post(group, input.Text, attachments); err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeBot(w http.ResponseWriter, bot botcore.Bot, err error) {
	if err != nil {
		httpapi.WriteError(w, errorStatus(err), err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, bot)
}

func errorStatus(err error) int {
	if errors.Is(err, storage.ErrBotNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
