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
)

const (
	codexPlanKindMetaKey  = "codex.plan_kind"
	codexPlanKindProposal = "proposal"
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
	var title string
	var currentTitle string
	var publishACP bool
	var messageChunk string
	var messageID string
	var thoughtChunk string
	var thoughtMessageID string
	var toolEvent *sessionevents.ACPToolCall
	var attention bool
	now := time.Now().UTC()
	update, err := acpschema.DecodeSessionUpdate(raw)
	if err != nil {
		return
	}
	recordTool := func(src sessionevents.ACPToolCall, create bool) {
		src.UpdatedAt = now
		if job.toolByID == nil {
			job.toolByID = make(map[string]sessionevents.ACPToolCall)
		}
		call, exists := job.toolByID[src.ID]
		if !exists && !create && !toolUpdateCanMaterialize(src) {
			if job.pendingToolUpdateByID == nil {
				job.pendingToolUpdateByID = make(map[string]sessionevents.ACPToolCall)
			}
			pending := job.pendingToolUpdateByID[src.ID]
			mergeToolCall(&pending, src)
			job.pendingToolUpdateByID[src.ID] = pending
			return
		}
		if pending, ok := job.pendingToolUpdateByID[src.ID]; ok {
			mergeToolCall(&call, pending)
			delete(job.pendingToolUpdateByID, src.ID)
		}
		if call.StartedAt.IsZero() {
			call.StartedAt = now
		}
		mergeToolCall(&call, src)
		if exists && job.toolByID[src.ID].EqualTranscript(call) {
			return
		}
		job.toolByID[src.ID] = call
		job.ToolCalls = sortedToolCalls(job.toolByID)
		job.LastToolAt = now
		toolEvent = &call
	}
	if m.applySideChatUpdate(job, update) {
		return
	}
	subagentUpdate := providerSubagentFromUpdate(job.ACPAgent, update)
	if subagentUpdate.subagent != nil {
		m.publishProviderSubagent(job.eventSnapshot(), *subagentUpdate.subagent)
	}
	if subagentUpdate.consume {
		return
	}
	job.mu.Lock()
	switch event := update.(type) {
	case acpschema.AgentMessageChunkUpdate:
		messageChunk = contentText(event.Content)
		messageID = event.MessageID
		job.appendAssistantLocked(messageChunk)
		if messageChunk != "" {
			job.savedAssistantLen = len(job.Assistant)
		}
	case acpschema.AgentThoughtChunkUpdate:
		thoughtChunk = contentText(event.Content)
		thoughtMessageID = event.MessageID
		job.appendThoughtLocked(thoughtChunk)
	case acpschema.ToolCallSessionUpdate:
		recordTool(toolUpdateSnapshot(toolUpdateFields{
			ID:        event.ToolCallID,
			Title:     event.Title,
			Status:    event.Status,
			Kind:      event.Kind,
			Content:   event.Content,
			Locations: event.Locations,
			RawInput:  event.RawInput,
			RawOutput: event.RawOutput,
			Meta:      event.Meta,
			At:        now,
		}), true)
	case acpschema.ToolCallUpdateSessionUpdate:
		recordTool(toolUpdateSnapshot(toolUpdateFields{
			ID:        event.ToolCallID,
			Title:     event.Title,
			Status:    event.Status,
			Kind:      event.Kind,
			Content:   event.Content,
			Locations: event.Locations,
			RawInput:  event.RawInput,
			RawOutput: event.RawOutput,
			Meta:      event.Meta,
			At:        now,
		}), false)
	case acpschema.PlanSessionUpdate:
		plan := make([]sessionevents.PlanEntry, 0, len(event.Entries))
		for _, entry := range event.Entries {
			plan = append(plan, sessionevents.PlanEntry{
				Content:  entry.Content,
				Status:   string(entry.Status),
				Priority: string(entry.Priority),
			})
		}
		acceptProposal := job.turn != nil && acceptsACPPlanProposal(job.turn.planRequested, job.ACPAgent)
		if acceptProposal {
			job.turn.planDocument = ""
		}
		planKind, _ := event.Meta[codexPlanKindMetaKey].(string)
		if planKind == codexPlanKindProposal {
			if acceptProposal && len(plan) == 1 {
				job.turn.planDocument = strings.TrimSpace(plan[0].Content)
			}
			if len(job.Plan) > 0 {
				job.Plan = sessionevents.PlanCleared
				publishACP = true
			}
			break
		}
		var ok bool
		plan, ok = sessionevents.NormalizeProgressEntries(plan)
		if !ok {
			if len(job.Plan) > 0 {
				job.Plan = sessionevents.PlanCleared
				publishACP = true
			}
			break
		}
		wasEmpty := len(job.Plan) == 0
		publishACP = !slices.Equal(job.Plan, plan)
		job.Plan = plan
		attention = publishACP && wasEmpty && len(plan) > 0
	case acpschema.CurrentModeSessionUpdate:
		publishACP = job.Modes.CurrentModeID != string(event.CurrentModeID)
		job.Modes.CurrentModeID = string(event.CurrentModeID)
	case acpschema.SessionInfoSessionUpdate:
		if nextTitle := strings.TrimSpace(event.Title); nextTitle != "" {
			currentTitle = job.Title
			title = nextTitle
		}
	}
	job.UpdatedAt = now
	job.LastEventAt = now
	sessionID := job.ID
	job.mu.Unlock()

	if attention {
		m.touchJobAttention(job)
	}
	if title != "" {
		if session, updated, err := m.store.UpdateSessionTitleFromRuntime(sessionID, title); err == nil {
			job.mu.Lock()
			job.Title = session.Title
			job.mu.Unlock()
			publishACP = updated && session.Title != currentTitle
		}
	}
	if messageChunk != "" {
		m.queueACPMessageWithID(job, messageChunk, messageID)
	}
	if thoughtChunk != "" {
		m.queueACPThoughtWithID(job, thoughtChunk, thoughtMessageID)
	}
	if toolEvent != nil {
		m.publishACPTool(job.eventSnapshot(), *toolEvent)
	}
	if publishACP {
		m.publishACP(job.eventSnapshot())
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

func toolUpdateCanMaterialize(call sessionevents.ACPToolCall) bool {
	return len(call.Content) > 0 || len(call.RawInput) > 0 || len(call.RawOutput) > 0 || len(call.Locations) > 0
}
