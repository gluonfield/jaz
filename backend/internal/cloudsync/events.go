package cloudsync

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func isCloudUserEvent(kind string) bool {
	switch kind {
	case "user_message", "turn.user", "turn_user", "turn.started", "turn_started":
		return true
	default:
		return false
	}
}

func isCloudAssistantDelta(kind string) bool {
	switch kind {
	case "assistant_delta", "assistant.delta", "agent_message_chunk":
		return true
	default:
		return false
	}
}

func isCloudAssistantMessage(kind string) bool {
	switch kind {
	case "assistant_message", "assistant.message", "turn.assistant", "turn_assistant":
		return true
	default:
		return false
	}
}

func isCloudThinkingDelta(kind string) bool {
	switch kind {
	case "thinking_delta", "thinking.delta", "reasoning_delta":
		return true
	default:
		return false
	}
}

func isCloudToolCall(kind string) bool {
	switch kind {
	case "tool_call", "tool.call":
		return true
	default:
		return false
	}
}

func isCloudToolDelta(kind string) bool {
	switch kind {
	case "tool_delta", "tool.delta":
		return true
	default:
		return false
	}
}

func isCloudToolResult(kind string) bool {
	switch kind {
	case "tool_result", "tool.result":
		return true
	default:
		return false
	}
}

func isCloudToolEvent(kind string) bool {
	return isCloudToolCall(kind) || isCloudToolDelta(kind) || isCloudToolResult(kind)
}

func cloudTerminalState(kind string) string {
	switch kind {
	case "turn_completed", "turn.completed":
		return "idle"
	case "turn_failed", "turn.failed":
		return "failed"
	case "turn_interrupted", "turn.interrupted":
		return "cancelled"
	default:
		return ""
	}
}

func payloadMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func payloadText(p map[string]any) string {
	for _, key := range []string{"text", "prompt", "content", "delta"} {
		if v, ok := p[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func payloadString(p map[string]any, key string) string {
	v, _ := p[key].(string)
	return v
}

func payloadToolCallID(p map[string]any) string {
	for _, key := range []string{"call_id", "id", "tool_call_id"} {
		if v := payloadString(p, key); v != "" {
			return v
		}
	}
	return ""
}

func payloadArguments(p map[string]any) (string, map[string]any) {
	raw, ok := p["arguments"]
	if !ok || raw == nil {
		return "", nil
	}
	if s, ok := raw.(string); ok {
		var m map[string]any
		_ = json.Unmarshal([]byte(s), &m)
		return s, m
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", nil
	}
	if m, ok := raw.(map[string]any); ok {
		return string(encoded), m
	}
	return string(encoded), map[string]any{"value": raw}
}

func normalizeCloudToolStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "completed", "done", "ok", "success":
		return "completed"
	case "failed", "error", "declined", "cancelled", "canceled":
		return "failed"
	default:
		return "in_progress"
	}
}

type cloudToolState struct {
	id         string
	tool       string
	title      string
	status     string
	inputJSON  string
	rawInput   map[string]any
	streamOut  strings.Builder
	finalOut   string
	errMsg     string
	startedAt  time.Time
	updatedAt  time.Time
}

func (ts *cloudToolState) resultText() string {
	out := ts.finalOut
	if out == "" {
		out = ts.streamOut.String()
	}
	if ts.errMsg == "" {
		return out
	}
	if out == "" {
		return ts.errMsg
	}
	return out + "\n" + ts.errMsg
}

func (ts *cloudToolState) toACPToolCall() sessionevents.ACPToolCall {
	title := ts.title
	if title == "" {
		title = ts.tool
	}
	call := sessionevents.ACPToolCall{
		ID:        ts.id,
		Title:     title,
		Status:    ts.status,
		ToolName:  ts.tool,
		RawInput:  ts.rawInput,
		StartedAt: ts.startedAt,
		UpdatedAt: ts.updatedAt,
	}
	text := ts.resultText()
	if text != "" {
		call.Content = []sessionevents.ACPToolContent{{Type: "text", Text: text}}
		if ts.tool == "commandExecution" {
			call.Runtime.TerminalOutput = text
		}
		if ts.status == "completed" || ts.status == "failed" {
			rawOut, _ := json.Marshal(map[string]any{
				"output": ts.finalOut,
				"error":  ts.errMsg,
			})
			call.RawOutput = rawOut
		}
	}
	return call
}

func applyCloudToolEvent(tools map[string]*cloudToolState, ev storage.CloudThreadEvent, p map[string]any) (*cloudToolState, bool) {
	callID := payloadToolCallID(p)
	if callID == "" {
		callID = fmt.Sprintf("tool-%d", ev.Seq)
	}
	ts, exists := tools[callID]
	if !exists {
		ts = &cloudToolState{
			id:        callID,
			status:    "in_progress",
			startedAt: ev.CreatedAt,
		}
		tools[callID] = ts
	}
	ts.updatedAt = ev.CreatedAt

	switch {
	case isCloudToolCall(ev.Type):
		if tool := payloadString(p, "tool"); tool != "" {
			ts.tool = tool
		}
		if title := payloadString(p, "title"); title != "" {
			ts.title = title
		}
		ts.status = normalizeCloudToolStatus(payloadString(p, "status"))
		if inputJSON, rawInput := payloadArguments(p); inputJSON != "" {
			ts.inputJSON = inputJSON
			ts.rawInput = rawInput
		}
		return ts, true

	case isCloudToolDelta(ev.Type):
		if delta := payloadText(p); delta != "" {
			ts.streamOut.WriteString(delta)
		}
		return ts, true

	case isCloudToolResult(ev.Type):
		if tool := payloadString(p, "tool"); tool != "" && ts.tool == "" {
			ts.tool = tool
		}
		if title := payloadString(p, "title"); title != "" && ts.title == "" {
			ts.title = title
		}
		statusRaw := payloadString(p, "status")
		if statusRaw == "" {
			statusRaw = "completed"
		}
		ts.status = normalizeCloudToolStatus(statusRaw)
		if out := payloadString(p, "output"); out != "" {
			ts.finalOut = out
		}
		if errMsg := payloadString(p, "error"); errMsg != "" {
			ts.errMsg = errMsg
			if ts.status == "in_progress" {
				ts.status = "failed"
			}
		}
		return ts, true
	}
	return nil, false
}

func makeACPThoughtEvent(sessionID string, ev storage.CloudThreadEvent, p map[string]any, runID int, chunk string) sessionevents.Event {
	runStr := strconv.Itoa(runID)
	return sessionevents.Event{
		Seq:           ev.Seq,
		SessionID:     sessionID,
		Type:          sessionevents.TypeACPThought,
		ProjectionKey: "acp_thought:" + sessionID + ":" + runStr,
		ProjectionOp:  sessionevents.ProjectionAppend,
		ACP: &sessionevents.ACPEvent{
			ID:          sessionID,
			Slug:        sessionID,
			Agent:       payloadString(p, "agent"),
			SessionID:   sessionID,
			State:       "running",
			Thought:     chunk,
			TextRunID:   runStr,
			LastEventAt: ev.CreatedAt,
		},
		At: ev.CreatedAt,
	}
}

func makeACPToolEvent(sessionID string, ev storage.CloudThreadEvent, p map[string]any, ts *cloudToolState) sessionevents.Event {
	call := ts.toACPToolCall()
	return sessionevents.Event{
		Seq:           ev.Seq,
		SessionID:     sessionID,
		Type:          "acp_tool",
		ProjectionKey: "acp_tool:" + sessionID + ":" + call.ID,
		ProjectionOp:  sessionevents.ProjectionReplace,
		ACP: &sessionevents.ACPEvent{
			ID:          sessionID,
			Slug:        sessionID,
			Agent:       payloadString(p, "agent"),
			SessionID:   sessionID,
			State:       "running",
			ToolCalls:   []sessionevents.ACPToolCall{call},
			LastEventAt: ev.CreatedAt,
			LastToolAt:  ev.CreatedAt,
		},
		At: ev.CreatedAt,
	}
}

func ProjectTranscript(thread storage.CloudThread, rawEvents []storage.CloudThreadEvent) ProjectedTranscript {
	session := thread.ToSession()
	messages := make([]storage.Message, 0)
	projector := sessionevents.NewProjector()
	var latestSeq int64
	acpState := "idle"
	if session.Status == storage.StatusRunning {
		acpState = "running"
	}
	var acpError string

	var (
		pendingText      strings.Builder
		pendingReasoning strings.Builder
		pendingBlocks    []storage.Block
		toolBlockIdx     = map[string]int{}
		toolStates       = map[string]*cloudToolState{}
		pendingSeq       int64
		pendingAt        time.Time
		thoughtRun       int
		inThought        bool
		lastTurnTerminal string
	)

	flushPending := func() {
		if len(pendingBlocks) == 0 && pendingText.Len() == 0 && pendingReasoning.Len() == 0 {
			return
		}
		blocks := make([]storage.Block, len(pendingBlocks))
		copy(blocks, pendingBlocks)
		messages = append(messages, storage.Message{
			ThreadID:  thread.ID,
			Seq:       pendingSeq,
			Role:      "assistant",
			Content:   pendingText.String(),
			Reasoning: pendingReasoning.String(),
			Blocks:    blocks,
			CreatedAt: pendingAt,
		})
		pendingText.Reset()
		pendingReasoning.Reset()
		pendingBlocks = nil
		toolBlockIdx = map[string]int{}
		toolStates = map[string]*cloudToolState{}
		pendingSeq = 0
		pendingAt = time.Time{}
		inThought = false
	}

	upsertToolBlock := func(ts *cloudToolState) {
		block := storage.Block{
			Type:      storage.BlockTypeTool,
			ID:        ts.id,
			Name:      ts.tool,
			InputJSON: ts.inputJSON,
			Result:    ts.resultText(),
		}
		if idx, ok := toolBlockIdx[ts.id]; ok {
			pendingBlocks[idx] = block
			return
		}
		toolBlockIdx[ts.id] = len(pendingBlocks)
		pendingBlocks = append(pendingBlocks, block)
	}

	for _, ev := range rawEvents {
		if ev.Seq > latestSeq {
			latestSeq = ev.Seq
		}
		p := payloadMap(ev.PayloadJSON)
		text := payloadText(p)

		if isCloudUserEvent(ev.Type) && text != "" {
			flushPending()
			lastTurnTerminal = ""
			acpError = ""
			messages = append(messages, storage.Message{
				ThreadID:         thread.ID,
				Seq:              ev.Seq,
				Role:             "user",
				FromUser:         true,
				ActorUserID:      ev.ActorUserID,
				ActorDisplayName: payloadString(p, "actor_display_name"),
				ActorEmail:       payloadString(p, "actor_email"),
				Content:          text,
				Blocks:           []storage.Block{{Type: storage.BlockTypeText, Text: text}},
				CreatedAt:        ev.CreatedAt,
			})
			continue
		}

		if isCloudThinkingDelta(ev.Type) {
			if text != "" {
				lastTurnTerminal = ""
				if !inThought {
					thoughtRun++
					inThought = true
				}
				pendingReasoning.WriteString(text)
				if n := len(pendingBlocks); n > 0 && pendingBlocks[n-1].Type == storage.BlockTypeReasoning {
					pendingBlocks[n-1].Text += text
				} else {
					pendingBlocks = append(pendingBlocks, storage.Block{
						Type: storage.BlockTypeReasoning,
						Text: text,
					})
				}
				pendingSeq = ev.Seq
				pendingAt = ev.CreatedAt
				projector.Apply(makeACPThoughtEvent(thread.ID, ev, p, thoughtRun, text))
			}
			continue
		}

		if isCloudToolEvent(ev.Type) {
			inThought = false
			lastTurnTerminal = ""
			if ts, ok := applyCloudToolEvent(toolStates, ev, p); ok {
				upsertToolBlock(ts)
				pendingSeq = ev.Seq
				pendingAt = ev.CreatedAt
				projector.Apply(makeACPToolEvent(thread.ID, ev, p, ts))
			}
			continue
		}

		if isCloudAssistantDelta(ev.Type) {
			inThought = false
			if text != "" {
				lastTurnTerminal = ""
				pendingText.WriteString(text)
				if n := len(pendingBlocks); n > 0 && pendingBlocks[n-1].Type == storage.BlockTypeText {
					pendingBlocks[n-1].Text += text
				} else {
					pendingBlocks = append(pendingBlocks, storage.Block{
						Type: storage.BlockTypeText,
						Text: text,
					})
				}
				pendingSeq = ev.Seq
				pendingAt = ev.CreatedAt
			}
			continue
		}

		if isCloudAssistantMessage(ev.Type) && text != "" {
			inThought = false
			lastTurnTerminal = ""
			pendingText.Reset()
			pendingReasoning.Reset()
			pendingBlocks = nil
			toolBlockIdx = map[string]int{}
			toolStates = map[string]*cloudToolState{}
			messages = append(messages, storage.Message{
				ThreadID:  thread.ID,
				Seq:       ev.Seq,
				Role:      "assistant",
				Content:   text,
				Blocks:    []storage.Block{{Type: storage.BlockTypeText, Text: text}},
				CreatedAt: ev.CreatedAt,
			})
			continue
		}

		inThought = false
		if term := cloudTerminalState(ev.Type); term != "" {
			flushPending()
			lastTurnTerminal = term
			acpState = term
			acpError = strings.TrimSpace(payloadString(p, "error"))
		}

		projector.Apply(cloudEventToSessionEvent(thread.ID, ev, ""))
	}

	if lastTurnTerminal != "" {
		acpState = lastTurnTerminal
		if session.Status == storage.StatusRunning {
			switch lastTurnTerminal {
			case "failed":
				session.Status = storage.StatusError
			case "cancelled":
				session.Status = storage.StatusInterrupted
			default:
				session.Status = storage.StatusIdle
			}
		}
	}

	var acpAssistant string
	if acpState == "running" && pendingText.Len() > 0 {
		acpAssistant = pendingText.String()
	} else if acpState != "running" {
		flushPending()
	}

	rev := latestSeq
	if rev < 1 {
		rev = 1
	}
	return ProjectedTranscript{
		Session:         session,
		Messages:        messages,
		Events:          projector.Events(),
		LatestSeq:       latestSeq,
		HistoryRevision: rev,
		ACPState:        acpState,
		ACPAssistant:    acpAssistant,
		ACPError:        acpError,
	}
}

type StreamProjector struct {
	threadID   string
	assistant  strings.Builder
	thoughtRun int
	inThought  bool
	tools      map[string]*cloudToolState
}

func NewStreamProjector(threadID string) *StreamProjector {
	return &StreamProjector{
		threadID: threadID,
		tools:    map[string]*cloudToolState{},
	}
}

func (sp *StreamProjector) Convert(ev storage.CloudThreadEvent) sessionevents.Event {
	sessionID := ev.ThreadID
	if sessionID == "" {
		sessionID = sp.threadID
	}
	p := payloadMap(ev.PayloadJSON)
	switch {
	case isCloudUserEvent(ev.Type):
		sp.assistant.Reset()
		sp.inThought = false
		sp.tools = map[string]*cloudToolState{}
	case isCloudThinkingDelta(ev.Type):
		if !sp.inThought {
			sp.thoughtRun++
			sp.inThought = true
		}
		return makeACPThoughtEvent(sessionID, ev, p, sp.thoughtRun, payloadText(p))
	case isCloudToolEvent(ev.Type):
		sp.inThought = false
		if sp.tools == nil {
			sp.tools = map[string]*cloudToolState{}
		}
		if ts, ok := applyCloudToolEvent(sp.tools, ev, p); ok {
			return makeACPToolEvent(sessionID, ev, p, ts)
		}
	case isCloudAssistantDelta(ev.Type):
		sp.inThought = false
		if chunk := payloadText(p); chunk != "" {
			sp.assistant.WriteString(chunk)
		}
		return cloudEventToSessionEvent(sp.threadID, ev, sp.assistant.String())
	case cloudTerminalState(ev.Type) != "":
		sp.assistant.Reset()
		sp.inThought = false
		sp.tools = map[string]*cloudToolState{}
	default:
		sp.inThought = false
	}
	return cloudEventToSessionEvent(sp.threadID, ev, "")
}

func cloudEventToSessionEvent(threadID string, ev storage.CloudThreadEvent, assistantOverride string) sessionevents.Event {
	sessionID := ev.ThreadID
	if sessionID == "" {
		sessionID = threadID
	}
	p := payloadMap(ev.PayloadJSON)
	if ev.Type == "acp" && p != nil && p["acp"] != nil {
		var full sessionevents.Event
		if json.Unmarshal(ev.PayloadJSON, &full) == nil {
			full.Seq = ev.Seq
			full.SessionID = sessionID
			full.Type = "acp"
			full.At = ev.CreatedAt
			return full
		}
	}
	if isCloudAssistantDelta(ev.Type) || isCloudAssistantMessage(ev.Type) {
		text := assistantOverride
		if text == "" {
			text = payloadText(p)
		}
		return sessionevents.Event{
			Seq:       ev.Seq,
			SessionID: sessionID,
			Type:      "acp",
			ACP: &sessionevents.ACPEvent{
				ID:          sessionID,
				Slug:        sessionID,
				Agent:       payloadString(p, "agent"),
				SessionID:   sessionID,
				State:       "running",
				Assistant:   text,
				LastEventAt: ev.CreatedAt,
			},
			At: ev.CreatedAt,
		}
	}
	if term := cloudTerminalState(ev.Type); term != "" {
		return sessionevents.Event{
			Seq:       ev.Seq,
			SessionID: sessionID,
			Type:      "acp",
			ACP: &sessionevents.ACPEvent{
				ID:          sessionID,
				Slug:        sessionID,
				Agent:       payloadString(p, "agent"),
				SessionID:   sessionID,
				State:       term,
				Error:       strings.TrimSpace(payloadString(p, "error")),
				LastEventAt: ev.CreatedAt,
			},
			At: ev.CreatedAt,
		}
	}
	return sessionevents.Event{
		Seq:       ev.Seq,
		SessionID: sessionID,
		Type:      ev.Type,
		Content:   payloadText(p),
		At:        ev.CreatedAt,
	}
}
