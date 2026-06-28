package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (s *Server) streamSessionEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.Events == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("session events are not configured"))
		return
	}
	mobile := requestClientPlatform(r) == "mobile"
	afterSeq, err := sessionEventsAfterSeq(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	events := s.Events.Subscribe(r.Context(), sessionID)
	stored, err := s.Store.LoadSessionEventsAfter(sessionID, afterSeq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	for _, event := range stored {
		if mobile {
			event = mobileSessionEvent(event)
		}
		writeSessionEventSSE(w, flusher, event)
		afterSeq = event.Seq
	}
	for event := range events {
		if event.Seq > 0 && event.Seq <= afterSeq {
			continue
		}
		if mobile {
			event = mobileSessionEvent(event)
		}
		writeSessionEventSSE(w, flusher, event)
		if event.Seq > afterSeq {
			afterSeq = event.Seq
		}
	}
}

func sessionEventsAfterSeq(r *http.Request) (int64, error) {
	seq, err := parseEventSeq(r.URL.Query().Get("after_seq"))
	if err != nil {
		return 0, fmt.Errorf("after_seq must be a non-negative integer: %w", err)
	}
	last, err := parseEventSeq(r.Header.Get("Last-Event-ID"))
	if err != nil {
		return 0, fmt.Errorf("Last-Event-ID must be a non-negative integer: %w", err)
	}
	if last > seq {
		return last, nil
	}
	return seq, nil
}

func parseEventSeq(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq < 0 {
		if err == nil {
			err = fmt.Errorf("negative")
		}
		return 0, err
	}
	return seq, nil
}

func writeSessionEventSSE(w http.ResponseWriter, flusher http.Flusher, event sessionevents.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if event.Seq > 0 {
		_, _ = fmt.Fprintf(w, "id: %d\n", event.Seq)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
