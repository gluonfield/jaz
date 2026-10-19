package bots

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	events := h.events.Subscribe(r.Context(), "", sessionevents.TypeSession, "acp", "acp_tool", "permission_request", "permission_response", sessionevents.TypeRoomMessage, sessionevents.TypeBotActivity)
	bots, err := h.bots.List()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	controller := http.NewResponseController(w)
	send := func(kind string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := send("snapshot", listResponse{Bots: bots}); err != nil {
		return
	}
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			for _, id := range h.bots.ChangedBots(event) {
				bot, err := h.bots.Load(id)
				if errors.Is(err, storage.ErrBotNotFound) {
					if send("removed", id) != nil {
						return
					}
					continue
				}
				if err != nil || send("changed", bot) != nil {
					return
				}
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil || controller.Flush() != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
