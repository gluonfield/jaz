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

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/sessionevents"
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
	case acpschema.ClientMethodElicitationCreate:
		return m.createElicitation(ctx, req.Params)
	case acpschema.ClientMethodElicitationComplete:
		return jsonrpc.EncodeResult(map[string]any{})
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
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	return m.awaitPermission(ctx, job, req)
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

func (m *Manager) applyUpdate(acpSessionID string, raw json.RawMessage) {
	job := m.jobByACP(acpSessionID)
	if job == nil {
		return
	}
	m.recordRawUsage(job, raw)
	var activity *storage.ActivityEntry
	var title string
	var publishACP bool
	var messageChunk string
	var bufferMessage bool
	var thoughtChunk string
	var toolEvent *sessionevents.ACPToolCall
	var attention bool
	now := time.Now().UTC()
	update, err := acpschema.DecodeSessionUpdate(raw)
	if err != nil {
		return
	}
	recordTool := func(src sessionevents.ACPToolCall) {
		call := job.toolByID[src.ID]
		if call.StartedAt.IsZero() {
			call.StartedAt = now
		}
		src.UpdatedAt = now
		mergeToolCall(&call, src)
		job.toolByID[src.ID] = call
		job.ToolCalls = sortedToolCalls(job.toolByID)
		job.LastToolAt = now
		toolEvent = &call
		activity = &storage.ActivityEntry{
			ID:     call.ID,
			Kind:   "tool",
			Text:   firstNonEmpty(call.Title, call.ID),
			Status: call.Status,
			At:     now,
		}
	}
	if m.applySideChatUpdate(job, update) {
		return
	}
	subagentUpdate := providerSubagentFromUpdate(job.ACPAgent, update)
	if subagentUpdate.subagent != nil {
		m.publishProviderSubagent(job.Snapshot(), *subagentUpdate.subagent)
	}
	if subagentUpdate.consume {
		return
	}
	job.mu.Lock()
	switch event := update.(type) {
	case acpschema.AgentMessageChunkUpdate:
		messageChunk = contentText(event.Content)
		job.Assistant = appendACPText(job.Assistant, messageChunk)
		bufferMessage = job.turn != nil && planTurnDefersResult(job.turn.planRequested, job.ACPAgent)
		if messageChunk != "" {
			job.savedAssistantLen = len(job.Assistant)
		}
	case acpschema.AgentThoughtChunkUpdate:
		thoughtChunk = contentText(event.Content)
		job.Thought = appendACPText(job.Thought, thoughtChunk)
	case acpschema.ToolCallSessionUpdate:
		recordTool(toolUpdateSnapshot(event.ToolCallID, event.Title, event.Status, event.Kind, event.Content, event.RawInput, event.Meta, now))
	case acpschema.ToolCallUpdateSessionUpdate:
		recordTool(toolUpdateSnapshot(event.ToolCallID, event.Title, event.Status, event.Kind, event.Content, event.RawInput, event.Meta, now))
	case acpschema.PlanSessionUpdate:
		plan := make([]sessionevents.PlanEntry, 0, len(event.Entries))
		for _, entry := range event.Entries {
			plan = append(plan, sessionevents.PlanEntry{
				Content:  entry.Content,
				Status:   string(entry.Status),
				Priority: string(entry.Priority),
			})
		}
		deferPlan := job.turn != nil && planTurnDefersResult(job.turn.planRequested, job.ACPAgent)
		if planText, ok := sessionevents.NormalizePlanDocumentText(plan); ok && deferPlan {
			job.turn.planProposal = &sessionevents.PlanEvent{
				Explanation:      planText,
				AwaitingApproval: true,
			}
			if len(job.Plan) > 0 {
				job.Plan = []sessionevents.PlanEntry{}
				publishACP = true
			}
			break
		}
		var ok bool
		plan, ok = sessionevents.NormalizeProgressEntries(plan)
		if !ok {
			if deferPlan {
				job.turn.planProposal = nil
			}
			if len(job.Plan) > 0 {
				job.Plan = []sessionevents.PlanEntry{}
				publishACP = true
			}
			break
		}
		if deferPlan {
			if len(plan) == 0 {
				job.turn.planProposal = nil
			} else {
				job.turn.planProposal = &sessionevents.PlanEvent{
					Plan:             clonePlanEntries(plan),
					AwaitingApproval: true,
				}
			}
		}
		wasEmpty := len(job.Plan) == 0
		publishACP = !slices.Equal(job.Plan, plan)
		job.Plan = plan
		attention = publishACP && wasEmpty && len(plan) > 0
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
	job.LastEventAt = now
	sessionID := job.ID
	job.mu.Unlock()

	if activity != nil {
		_ = m.store.UpsertActivity(sessionID, *activity)
	}
	if attention {
		m.touchJobAttention(job)
	}
	if title != "" {
		if session, err := m.store.LoadSession(sessionID); err == nil {
			session.Title = title
			_ = m.store.SaveSession(session)
		}
	}
	if messageChunk != "" && !bufferMessage {
		m.queueACPMessage(job, messageChunk)
	}
	if thoughtChunk != "" {
		m.queueACPThought(job, thoughtChunk)
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
	return existing + chunk
}

func sortedToolCalls(in map[string]sessionevents.ACPToolCall) []sessionevents.ACPToolCall {
	out := make([]sessionevents.ACPToolCall, 0, len(in))
	for _, call := range in {
		out = append(out, call)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
