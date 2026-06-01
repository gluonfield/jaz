package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	acpschema "github.com/wins/acp-transport/acp"
	"github.com/wins/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) handleJSONRPC(ctx context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
	switch req.Method {
	case acpschema.ClientMethodSessionUpdate:
		var note struct {
			SessionID string          `json:"sessionId"`
			Update    json.RawMessage `json:"update"`
		}
		if err := json.Unmarshal(req.Params, &note); err != nil {
			return nil, jsonrpc.InvalidParams("invalid session/update", map[string]any{"error": err.Error()})
		}
		m.applyUpdate(note.SessionID, note.Update)
		return jsonrpc.EncodeResult(map[string]any{})
	case acpschema.ClientMethodSessionRequestPermission:
		return jsonrpc.EncodeResult(map[string]any{"outcome": "cancelled"})
	case acpschema.ClientMethodFSReadTextFile:
		return m.readTextFile(req.Params)
	case acpschema.ClientMethodFSWriteTextFile:
		return m.writeTextFile(req.Params)
	case acpschema.ClientMethodTerminalKill, acpschema.ClientMethodTerminalRelease:
		return jsonrpc.EncodeResult(map[string]any{})
	case acpschema.ClientMethodTerminalCreate, acpschema.ClientMethodTerminalOutput, acpschema.ClientMethodTerminalWaitForExit:
		return nil, jsonrpc.InternalError("terminal support is disabled", nil)
	default:
		return nil, jsonrpc.MethodNotFound(req.Method)
	}
}

func (m *Manager) readTextFile(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.ReadTextFileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid fs/read_text_file", map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	path, err := safePath(job.Cwd, req.Path)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	content := string(data)
	if req.Limit > 0 && len(content) > req.Limit {
		content = content[:req.Limit]
	}
	return jsonrpc.EncodeResult(acpschema.ReadTextFileResponse{Content: content})
}

func (m *Manager) writeTextFile(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.WriteTextFileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid fs/write_text_file", map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	path, err := safePath(job.Cwd, req.Path)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	return jsonrpc.EncodeResult(acpschema.WriteTextFileResponse{})
}

func (m *Manager) applyUpdate(acpSessionID string, raw json.RawMessage) {
	job := m.jobByACP(acpSessionID)
	if job == nil {
		return
	}
	var env struct {
		SessionUpdate string          `json:"sessionUpdate"`
		Content       json.RawMessage `json:"content"`
		Title         string          `json:"title"`
		ToolCallID    string          `json:"toolCallId"`
		Status        json.RawMessage `json:"status"`
		Entries       []struct {
			Content  string          `json:"content"`
			Status   json.RawMessage `json:"status"`
			Priority json.RawMessage `json:"priority"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	var activity *storage.ActivityEntry
	var title string
	now := time.Now().UTC()
	job.mu.Lock()
	switch env.SessionUpdate {
	case "agent_message_chunk":
		job.Assistant += contentText(env.Content)
	case "agent_thought_chunk":
		job.Thought += contentText(env.Content)
	case "tool_call", "tool_call_update":
		call := job.toolByID[env.ToolCallID]
		call.ID = env.ToolCallID
		if env.Title != "" {
			call.Title = env.Title
		}
		if len(env.Status) > 0 {
			call.Status = rawString(env.Status)
		}
		job.toolByID[env.ToolCallID] = call
		job.ToolCalls = sortedToolCalls(job.toolByID)
		activity = &storage.ActivityEntry{
			ID:     call.ID,
			Kind:   "tool",
			Text:   firstNonEmpty(call.Title, call.ID),
			Status: call.Status,
			At:     now,
		}
	case "plan":
		job.Plan = make([]PlanEntry, 0, len(env.Entries))
		for _, entry := range env.Entries {
			job.Plan = append(job.Plan, PlanEntry{
				Content:  entry.Content,
				Status:   rawString(entry.Status),
				Priority: rawString(entry.Priority),
			})
		}
	case "session_info_update":
		if env.Title != "" {
			job.Title = env.Title
			title = env.Title
		}
	}
	job.UpdatedAt = now
	sessionID := job.ID
	job.mu.Unlock()

	if activity != nil {
		_ = m.store.UpsertActivity(sessionID, *activity)
	}
	if title != "" {
		if session, err := m.store.LoadSession(sessionID); err == nil {
			session.Title = title
			_ = m.store.SaveSession(session)
		}
	}
}

func contentText(raw json.RawMessage) string {
	var block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &block); err == nil && block.Type == "text" {
		return block.Text
	}
	return ""
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

func sortedToolCalls(in map[string]ToolCallSnapshot) []ToolCallSnapshot {
	out := make([]ToolCallSnapshot, 0, len(in))
	for _, call := range in {
		out = append(out, call)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
