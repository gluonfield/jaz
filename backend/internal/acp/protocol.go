package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/pathsafe"
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
		return m.requestPermission(ctx, req.Params)
	case acpschema.ClientMethodFSReadTextFile:
		return m.readTextFile(req.Params)
	case acpschema.ClientMethodFSWriteTextFile:
		return m.writeTextFile(req.Params)
	case acpschema.ClientMethodTerminalKill, acpschema.ClientMethodTerminalRelease:
		return jsonrpc.EncodeResult(map[string]any{})
	case acpschema.ClientMethodTerminalCreate, acpschema.ClientMethodTerminalOutput, acpschema.ClientMethodTerminalWaitForExit:
		return nil, jsonrpc.InternalError("terminal support is disabled", nil)
	case ClientMethodWidgetPublish:
		return m.widgetPublish(req.Params)
	default:
		return nil, jsonrpc.MethodNotFound(req.Method)
	}
}

func (m *Manager) requestPermission(ctx context.Context, raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.RequestPermissionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid session/request_permission", map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil || !locationsStayInside(job.Cwd, req.ToolCall.Locations) {
		return permissionCancelled()
	}
	if optionID := autoApprovedPermissionOption(job, req); optionID != "" {
		return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseSelected(acpschema.PermissionOptionID(optionID)))
	}
	job.mu.RLock()
	interactive := job.interactive
	job.mu.RUnlock()
	if interactive {
		return m.awaitPermission(ctx, job, req)
	}
	if len(req.ToolCall.Locations) == 0 {
		return permissionCancelled()
	}
	optionID := selectPermissionOption(req.Options)
	if optionID == "" {
		return permissionCancelled()
	}
	return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseSelected(acpschema.PermissionOptionID(optionID)))
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
	path, err := pathsafe.Resolve(job.Cwd, req.Path)
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
	path, err := pathsafe.Resolve(job.Cwd, req.Path)
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

func permissionCancelled() (json.RawMessage, *jsonrpc.Error) {
	return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseCancelled())
}

func selectPermissionOption(options []acpschema.PermissionOption) string {
	for _, option := range options {
		if permissionOptionAllows(option) {
			return string(option.OptionID)
		}
	}
	return ""
}

func autoApprovedPermissionOption(job *Job, req acpschema.RequestPermissionRequest) string {
	if CanonicalAgentName(job.ACPAgent) != AgentGrok {
		return ""
	}
	if len(codexUserInputQuestions(req)) > 0 {
		return ""
	}
	return selectPermissionOption(req.Options)
}

func permissionOptionAllows(option acpschema.PermissionOption) bool {
	switch option.Kind {
	case acpschema.PermissionOptionKindAllowAlways, acpschema.PermissionOptionKindAllowOnce:
		return true
	}
	text := strings.ToLower(string(option.OptionID) + " " + option.Name)
	return strings.Contains(text, "allow") || strings.Contains(text, "approve")
}

func locationsStayInside(root string, locations []acpschema.ToolCallLocation) bool {
	for _, location := range locations {
		if _, err := pathsafe.Resolve(root, location.Path); err != nil {
			return false
		}
	}
	return true
}

func (m *Manager) applyUpdate(acpSessionID string, raw json.RawMessage) {
	job := m.jobByACP(acpSessionID)
	if job == nil {
		return
	}
	if usage := usageFromRaw(raw); !usage.IsZero() {
		m.recordUsage(job, usage)
	}
	var activity *storage.ActivityEntry
	var title string
	var publishACP bool
	var messageChunk string
	var thoughtChunk string
	var toolEvent *ToolCallSnapshot
	now := time.Now().UTC()
	update, err := acpschema.DecodeSessionUpdate(raw)
	if err != nil {
		return
	}
	job.mu.Lock()
	switch event := update.(type) {
	case acpschema.AgentMessageChunkUpdate:
		messageChunk = contentText(event.Content)
		job.Assistant = appendACPText(job.Assistant, messageChunk)
		if messageChunk != "" {
			job.savedAssistantLen = len(job.Assistant)
		}
	case acpschema.AgentThoughtChunkUpdate:
		thoughtChunk = contentText(event.Content)
		job.Thought = appendACPText(job.Thought, thoughtChunk)
	case acpschema.ToolCallSessionUpdate:
		call := job.toolByID[string(event.ToolCallID)]
		call.ID = string(event.ToolCallID)
		if event.Title != "" {
			call.Title = event.Title
		}
		if event.Status != nil {
			call.Status = string(*event.Status)
		}
		job.toolByID[string(event.ToolCallID)] = call
		job.ToolCalls = sortedToolCalls(job.toolByID)
		toolEvent = &call
		activity = &storage.ActivityEntry{
			ID:     call.ID,
			Kind:   "tool",
			Text:   firstNonEmpty(call.Title, call.ID),
			Status: call.Status,
			At:     now,
		}
	case acpschema.ToolCallUpdateSessionUpdate:
		call := job.toolByID[string(event.ToolCallID)]
		call.ID = string(event.ToolCallID)
		if event.Title != "" {
			call.Title = event.Title
		}
		if event.Status != nil {
			call.Status = string(*event.Status)
		}
		job.toolByID[string(event.ToolCallID)] = call
		job.ToolCalls = sortedToolCalls(job.toolByID)
		toolEvent = &call
		activity = &storage.ActivityEntry{
			ID:     call.ID,
			Kind:   "tool",
			Text:   firstNonEmpty(call.Title, call.ID),
			Status: call.Status,
			At:     now,
		}
	case acpschema.PlanSessionUpdate:
		plan := make([]PlanEntry, 0, len(event.Entries))
		for _, entry := range event.Entries {
			plan = append(plan, PlanEntry{
				Content:  entry.Content,
				Status:   string(entry.Status),
				Priority: string(entry.Priority),
			})
		}
		// Agents re-send the plan constantly; only persist actual changes.
		publishACP = !slices.Equal(job.Plan, plan)
		job.Plan = plan
	case acpschema.CurrentModeSessionUpdate:
		publishACP = job.Modes.CurrentModeID != string(event.CurrentModeID)
		job.Modes.CurrentModeID = string(event.CurrentModeID)
	case acpschema.SessionInfoSessionUpdate:
		if nextTitle := strings.TrimSpace(event.Title); nextTitle != "" && nextTitle != job.Title {
			job.Title = nextTitle
			title = nextTitle
			publishACP = true
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
	if messageChunk != "" {
		m.publishACPMessage(job.Snapshot(), messageChunk)
	}
	if thoughtChunk != "" {
		m.publishACPThought(job.Snapshot(), thoughtChunk)
	}
	if toolEvent != nil {
		m.publishACPTool(job.Snapshot(), *toolEvent)
	}
	if publishACP {
		m.publishACP(job.Snapshot())
	}
}

func contentText(raw acpschema.ContentBlock) string {
	block, err := acpschema.DecodeContentBlock(raw)
	if err != nil {
		return ""
	}
	if text, ok := block.(acpschema.TextContentBlock); ok {
		return text.Text
	}
	return ""
}

func appendACPText(existing, chunk string) string {
	if chunk == "" {
		return existing
	}
	if existing == "" {
		return chunk
	}
	if startsOrEndsWhitespace(existing, chunk) || !looksLikeMessageBoundary(existing, chunk) {
		return existing + chunk
	}
	return existing + "\n\n" + chunk
}

func startsOrEndsWhitespace(existing, chunk string) bool {
	last, _ := utf8.DecodeLastRuneInString(existing)
	first, _ := utf8.DecodeRuneInString(chunk)
	return unicode.IsSpace(last) || unicode.IsSpace(first)
}

func looksLikeMessageBoundary(existing, chunk string) bool {
	last, _ := utf8.DecodeLastRuneInString(existing)
	first, _ := utf8.DecodeRuneInString(chunk)
	if !unicode.IsUpper(first) && first != '`' {
		return false
	}
	switch last {
	case '.', '!', '?', ':', '`', ')':
		return true
	default:
		return false
	}
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
