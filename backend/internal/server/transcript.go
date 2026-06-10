package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

// maxToolDetailChars bounds per-tool detail regardless of the requested size.
const maxToolDetailChars = 4000

type TranscriptMessage struct {
	Role      string           `json:"role"`
	Text      string           `json:"text,omitempty"`
	Tools     []TranscriptTool `json:"tools,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

type TranscriptTool struct {
	Name   string `json:"name,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// GET /v1/sessions/{ref}/transcript returns user/assistant text only.
// max_tool_chars=N additionally includes tool calls compressed to N characters.
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
		"messages": transcriptFromRecords(records, maxToolChars),
	}
	if counts := toolCounts(records); len(counts) > 0 {
		response["tool_counts"] = counts
	}
	writeJSON(w, http.StatusOK, response)
}

// toolCounts aggregates tool usage by name across the whole session, so callers
// see what the agent actually used without carrying per-call payloads.
func toolCounts(records []storage.Message) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		for _, block := range record.Blocks {
			if block.Type != storage.BlockTypeTool {
				continue
			}
			name := block.Name
			if name == "" {
				name = "unknown"
			}
			counts[name]++
		}
	}
	return counts
}

// transcriptFromRecords keeps user/assistant text and drops reasoning, attachments,
// and system messages. Tool blocks are dropped unless maxToolChars > 0, in which
// case each becomes a single compressed line.
func transcriptFromRecords(records []storage.Message, maxToolChars int) []TranscriptMessage {
	messages := make([]TranscriptMessage, 0, len(records))
	for _, record := range records {
		if record.Role != "user" && record.Role != "assistant" {
			continue
		}
		message := TranscriptMessage{
			Role:      record.Role,
			Text:      transcriptText(record),
			CreatedAt: record.CreatedAt,
		}
		if maxToolChars > 0 {
			message.Tools = compressedTools(record.Blocks, maxToolChars)
		}
		if message.Text == "" && len(message.Tools) == 0 {
			continue
		}
		messages = append(messages, message)
	}
	return messages
}

func transcriptText(record storage.Message) string {
	if text := strings.TrimSpace(record.Content); text != "" {
		return text
	}
	var parts []string
	for _, block := range record.Blocks {
		if block.Type != storage.BlockTypeText {
			continue
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func compressedTools(blocks []storage.Block, maxChars int) []TranscriptTool {
	var tools []TranscriptTool
	for _, block := range blocks {
		if block.Type != storage.BlockTypeTool {
			continue
		}
		detail := strings.TrimSpace(block.InputJSON)
		if result := strings.TrimSpace(block.Result); result != "" {
			if detail != "" {
				detail += " -> "
			}
			detail += result
		}
		detail = strings.Join(strings.Fields(detail), " ")
		if len(detail) > maxChars {
			detail = detail[:maxChars] + "..."
		}
		tools = append(tools, TranscriptTool{Name: block.Name, Detail: detail})
	}
	return tools
}
