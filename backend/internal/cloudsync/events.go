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

func isCloudTurnQueuedEvent(kind string) bool {
	return kind == "turn_queued" || kind == "turn.queued"
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
	id        string
	tool      string
	kind      string
	title     string
	status    string
	cwd       string
	exitCode  *int
	inputJSON string
	rawInput  map[string]any
	locations []sessionevents.ACPToolLocation
	diffs     []sessionevents.ACPToolContent
	streamOut strings.Builder
	finalOut  string
	errMsg    string
	startedAt time.Time
	updatedAt time.Time
}

func (ts *cloudToolState) inferKind() string {
	if ts.kind != "" {
		return ts.kind
	}
	switch strings.ToLower(strings.TrimSpace(ts.tool)) {
	case "commandexecution", "bash", "exec_command":
		return "execute"
	case "filechange", "apply_patch", "edit", "write":
		return "edit"
	case "websearch", "web_search":
		return "search"
	}
	norm := strings.ToLower(ts.tool)
	switch {
	case strings.Contains(norm, "read") || strings.Contains(norm, "list_dir"):
		return "read"
	case strings.Contains(norm, "search") || strings.Contains(norm, "grep") || strings.Contains(norm, "find"):
		return "search"
	case strings.Contains(norm, "edit") || strings.Contains(norm, "write"):
		return "edit"
	case strings.Contains(norm, "exec") || strings.Contains(norm, "command"):
		return "execute"
	case strings.Contains(norm, "fetch"):
		return "fetch"
	default:
		return ""
	}
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
		Kind:      ts.inferKind(),
		ToolName:  ts.tool,
		Locations: ts.locations,
		RawInput:  ts.rawInput,
		StartedAt: ts.startedAt,
		UpdatedAt: ts.updatedAt,
	}
	if len(ts.diffs) > 0 {
		call.Content = append(call.Content, ts.diffs...)
	}
	text := ts.resultText()
	if text != "" {
		if len(ts.diffs) == 0 {
			call.Content = append(call.Content, sessionevents.ACPToolContent{Type: "text", Text: text})
		}
		if ts.status == "completed" || ts.status == "failed" {
			rawOut, _ := json.Marshal(map[string]any{
				"output": ts.finalOut,
				"error":  ts.errMsg,
			})
			call.RawOutput = rawOut
		}
	}
	if call.Kind == "execute" || ts.cwd != "" || ts.exitCode != nil {
		call.Runtime.TerminalCwd = ts.cwd
		call.Runtime.TerminalOutput = text
		call.Runtime.TerminalExitCode = ts.exitCode
	}
	return call
}

func payloadLocations(p map[string]any) []sessionevents.ACPToolLocation {
	raw, ok := p["locations"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPToolLocation, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if pth, _ := m["path"].(string); strings.TrimSpace(pth) != "" {
			loc := sessionevents.ACPToolLocation{Path: strings.TrimSpace(pth)}
			if ln, ok := m["line"].(float64); ok && ln > 0 {
				loc.Line = int(ln)
			}
			out = append(out, loc)
		}
	}
	return out
}

func payloadDiffs(p map[string]any) []sessionevents.ACPToolContent {
	raw, ok := p["diffs"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPToolContent, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pth, _ := m["path"].(string)
		newText, _ := m["new_text"].(string)
		oldText, _ := m["old_text"].(string)
		if pth != "" || newText != "" {
			out = append(out, sessionevents.ACPToolContent{
				Type:    "diff",
				Path:    pth,
				OldText: oldText,
				NewText: newText,
			})
		}
	}
	return out
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
	if kind := payloadString(p, "kind"); kind != "" {
		ts.kind = kind
	}
	if cwd := payloadString(p, "cwd"); cwd != "" {
		ts.cwd = cwd
	}
	if locs := payloadLocations(p); len(locs) > 0 {
		ts.locations = locs
	}
	if diffs := payloadDiffs(p); len(diffs) > 0 {
		ts.diffs = diffs
	}
	if ec, ok := p["exit_code"].(float64); ok {
		v := int(ec)
		ts.exitCode = &v
	}

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

type cloudTurnState struct {
	threadID   string
	assistant  strings.Builder
	thoughtRun int
	inThought  bool
	needSep    bool
	tools      map[string]*cloudToolState
}

func newCloudTurnState(threadID string) cloudTurnState {
	return cloudTurnState{
		threadID: threadID,
		tools:    map[string]*cloudToolState{},
	}
}

func (st *cloudTurnState) resetTurn() {
	st.assistant.Reset()
	st.inThought = false
	st.needSep = false
	st.tools = map[string]*cloudToolState{}
}

func (st *cloudTurnState) onThought(sessionID string, ev storage.CloudThreadEvent, p map[string]any, chunk string) sessionevents.Event {
	if st.assistant.Len() > 0 {
		st.needSep = true
	}
	if !st.inThought {
		st.thoughtRun++
		st.inThought = true
	}
	return makeACPThoughtEvent(sessionID, ev, p, st.thoughtRun, chunk)
}

func (st *cloudTurnState) onTool(sessionID string, ev storage.CloudThreadEvent, p map[string]any) (sessionevents.Event, bool) {
	st.inThought = false
	if st.assistant.Len() > 0 {
		st.needSep = true
	}
	if st.tools == nil {
		st.tools = map[string]*cloudToolState{}
	}
	ts, ok := applyCloudToolEvent(st.tools, ev, p)
	if !ok {
		return sessionevents.Event{}, false
	}
	return makeACPToolEvent(sessionID, ev, p, ts), true
}

func (st *cloudTurnState) onAssistantDelta(chunk string) {
	st.inThought = false
	if chunk == "" {
		return
	}
	if st.needSep && st.assistant.Len() > 0 && !strings.HasSuffix(st.assistant.String(), "\n") && !strings.HasPrefix(chunk, "\n") {
		st.assistant.WriteString("\n\n")
	}
	st.needSep = false
	st.assistant.WriteString(chunk)
}

func ProjectTranscript(thread storage.CloudThread, rawEvents []storage.CloudThreadEvent) ProjectedTranscript {
	session := thread.ToSession()
	messages := make([]storage.Message, 0)
	projector := sessionevents.NewProjector()
	turn := newCloudTurnState(thread.ID)
	var latestSeq int64
	acpState := "idle"
	if session.Status == storage.StatusRunning {
		acpState = "running"
	}
	var acpError string

	var (
		pendingSeq       int64
		pendingAt        time.Time
		hasOpenTurn      bool
		lastTurnTerminal string
	)

	flushPending := func() {
		if turn.assistant.Len() > 0 {
			text := turn.assistant.String()
			messages = append(messages, storage.Message{
				ThreadID:  thread.ID,
				Seq:       pendingSeq,
				Role:      "assistant",
				Content:   text,
				Blocks:    []storage.Block{{Type: storage.BlockTypeText, Text: text}},
				CreatedAt: pendingAt,
			})
		}
		turn.resetTurn()
		pendingSeq = 0
		pendingAt = time.Time{}
	}

	for _, ev := range rawEvents {
		if ev.Seq > latestSeq {
			latestSeq = ev.Seq
		}
		p := payloadMap(ev.PayloadJSON)
		text := payloadText(p)

		if isCloudUserEvent(ev.Type) {
			if text != "" {
				flushPending()
				hasOpenTurn = true
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
			}
			continue
		}

		if isCloudTurnQueuedEvent(ev.Type) {
			continue
		}

		if isCloudThinkingDelta(ev.Type) {
			if text != "" {
				hasOpenTurn = true
				lastTurnTerminal = ""
				projector.Apply(turn.onThought(thread.ID, ev, p, text))
			}
			continue
		}

		if isCloudToolEvent(ev.Type) {
			hasOpenTurn = true
			lastTurnTerminal = ""
			if toolEv, ok := turn.onTool(thread.ID, ev, p); ok {
				projector.Apply(toolEv)
			}
			continue
		}

		if isCloudAssistantDelta(ev.Type) {
			if text != "" {
				hasOpenTurn = true
				lastTurnTerminal = ""
				turn.onAssistantDelta(text)
				pendingSeq = ev.Seq
				pendingAt = ev.CreatedAt
			}
			continue
		}

		if isCloudAssistantMessage(ev.Type) && text != "" {
			turn.resetTurn()
			hasOpenTurn = false
			lastTurnTerminal = ""
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

		turn.inThought = false
		if term := cloudTerminalState(ev.Type); term != "" {
			flushPending()
			hasOpenTurn = false
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
	} else if hasOpenTurn && session.Status == storage.StatusIdle {
		session.Status = storage.StatusRunning
		acpState = "running"
	}

	var acpAssistant string
	if acpState == "running" && turn.assistant.Len() > 0 {
		acpAssistant = turn.assistant.String()
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
	turn cloudTurnState
}

func NewStreamProjector(threadID string) *StreamProjector {
	return &StreamProjector{turn: newCloudTurnState(threadID)}
}

func cloudSessionRefreshEvent(sessionID string, ev storage.CloudThreadEvent) sessionevents.Event {
	return sessionevents.Event{
		Seq:       ev.Seq,
		SessionID: sessionID,
		Type:      "session",
		At:        ev.CreatedAt,
	}
}

func (sp *StreamProjector) Convert(ev storage.CloudThreadEvent) sessionevents.Event {
	sessionID := ev.ThreadID
	if sessionID == "" {
		sessionID = sp.turn.threadID
	}
	p := payloadMap(ev.PayloadJSON)
	switch {
	case isCloudUserEvent(ev.Type):
		sp.turn.resetTurn()
		return cloudSessionRefreshEvent(sessionID, ev)
	case isCloudTurnQueuedEvent(ev.Type):
		return cloudSessionRefreshEvent(sessionID, ev)
	case isCloudThinkingDelta(ev.Type):
		return sp.turn.onThought(sessionID, ev, p, payloadText(p))
	case isCloudToolEvent(ev.Type):
		if toolEv, ok := sp.turn.onTool(sessionID, ev, p); ok {
			return toolEv
		}
	case isCloudAssistantDelta(ev.Type):
		sp.turn.onAssistantDelta(payloadText(p))
		return cloudEventToSessionEvent(sp.turn.threadID, ev, sp.turn.assistant.String())
	case cloudTerminalState(ev.Type) != "":
		sp.turn.resetTurn()
	default:
		sp.turn.inThought = false
	}
	return cloudEventToSessionEvent(sp.turn.threadID, ev, "")
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
	if isCloudUserEvent(ev.Type) || isCloudTurnQueuedEvent(ev.Type) {
		return cloudSessionRefreshEvent(sessionID, ev)
	}
	return sessionevents.Event{
		Seq:       ev.Seq,
		SessionID: sessionID,
		Type:      ev.Type,
		Content:   payloadText(p),
		At:        ev.CreatedAt,
	}
}
