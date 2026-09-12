package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/google/uuid"
)

type grokInterjection struct {
	applied bool
	result  chan string
}

func (m *Manager) runGrokInterject(ctx context.Context, job *jobState, done chan struct{}, prompt acpschema.PromptRequest) {
	id := uuid.NewString()
	request := &grokInterjection{result: make(chan string, 1)}
	job.mu.Lock()
	if job.turn == nil || job.turn.done != done {
		job.mu.Unlock()
		return
	}
	if job.turn.grokInterjections == nil {
		job.turn.grokInterjections = make(map[string]*grokInterjection)
	}
	job.turn.grokInterjections[id] = request
	job.mu.Unlock()
	m.mu.RLock()
	process := m.processes[job.ID]
	m.mu.RUnlock()
	if process == nil {
		m.failPromptCall(done, job, fmt.Errorf("acp peer is not active"))
		return
	}
	var text strings.Builder
	for _, block := range prompt.Prompt {
		text.WriteString(contentText(block))
	}
	raw, err := process.peer.Call(ctx, string(steerGrokInterject), struct {
		SessionID      acpschema.SessionID      `json:"sessionId"`
		InterjectionID string                   `json:"interjectionId"`
		Text           string                   `json:"text"`
		Content        []acpschema.ContentBlock `json:"content"`
	}{prompt.SessionID, id, text.String(), prompt.Prompt})
	var response struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err == nil && (json.Unmarshal(raw, &response) != nil || response.Result.Status != "queued") {
		err = fmt.Errorf("invalid Grok interjection acknowledgement")
	}
	if err != nil {
		m.failPromptCall(done, job, err)
		return
	}
	// Grok acknowledges intake immediately; the native turn owns completion,
	// including a fallback turn if the previous prompt finished during admission.
	var reason string
	select {
	case reason = <-request.result:
	case <-done:
		return
	case <-process.closed:
		select {
		case reason = <-request.result:
		case <-done:
			return
		case <-ctx.Done():
			m.failPromptCall(done, job, ctx.Err())
			return
		case <-time.After(100 * time.Millisecond):
			m.failPromptCall(done, job, jsonrpc.ErrClosed)
			return
		}
	case <-ctx.Done():
		m.failPromptCall(done, job, ctx.Err())
		return
	}
	m.completePromptCall(done, job, reason)
}

func (m *Manager) grokInterjectionEvent(req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
	var note struct {
		SessionID      string          `json:"sessionId"`
		InterjectionID string          `json:"interjectionId"`
		Update         json.RawMessage `json:"update"`
	}
	if json.Unmarshal(req.Params, &note) != nil {
		return nil, jsonrpc.InvalidParams("invalid Grok session notification", nil)
	}
	job := m.jobByACP(note.SessionID)
	if job == nil || job.steerMethod != steerGrokInterject {
		return jsonrpc.EncodeResult(struct{}{})
	}
	var update struct {
		Kind       string `json:"sessionUpdate"`
		PromptID   string `json:"prompt_id"`
		StopReason string `json:"stop_reason"`
	}
	if req.Method == "_x.ai/session_notification" {
		if json.Unmarshal(note.Update, &update) != nil || update.Kind != "turn_completed" || update.StopReason == "" {
			return jsonrpc.EncodeResult(struct{}{})
		}
		report := usageReportFromRaw(note.Update)
		report.ID = update.PromptID
		m.recordUsageReport(job, report)
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.turn != nil {
		if req.Method == "_x.ai/session/interjection" {
			if request := job.turn.grokInterjections[note.InterjectionID]; request != nil {
				request.applied = true
			}
		} else {
			delivered := false
			for id, request := range job.turn.grokInterjections {
				if request.applied {
					request.result <- update.StopReason
					delete(job.turn.grokInterjections, id)
					delivered = true
				}
			}
			if delivered {
				job.turn.grokStopReason = update.StopReason
			}
		}
	}
	return jsonrpc.EncodeResult(struct{}{})
}
