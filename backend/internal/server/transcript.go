package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
)

const maxToolDetailChars = 4000

func (s *Server) writeSessionTranscript(w http.ResponseWriter, r *http.Request, session storage.Session) {
	maxToolChars := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("max_tool_chars")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("max_tool_chars must be a non-negative integer"))
			return
		}
		maxToolChars = min(parsed, maxToolDetailChars)
	}
	recordStore, ok := s.Store.(messageRecordStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("session store does not support transcripts"))
		return
	}
	records, err := recordStore.LoadMessageRecords(session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response := map[string]any{
		"session":  canonicalSessionResponse(session),
		"messages": threads.TranscriptFromRecords(records, maxToolChars),
	}
	if counts := threads.ToolCounts(records); len(counts) > 0 {
		response["tool_counts"] = counts
	}
	writeJSON(w, http.StatusOK, response)
}
