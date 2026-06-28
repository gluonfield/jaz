package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	codexRequestUserInputMetaKey  = "codex.request_user_input"
	userInputResponseOptionPrefix = "__user_input_response__:"
)

type pendingPermission struct {
	sessionID                     string
	request                       sessionevents.ACPPermission
	userInputResponseOptionPrefix string
	answer                        chan string
}

type InteractiveAnswerValue struct {
	Answers []string `json:"answers"`
}

func (m *Manager) awaitPermission(ctx context.Context, job *jobState, req acpschema.RequestPermissionRequest) (json.RawMessage, *jsonrpc.Error) {
	permission := permissionEvent(req)
	permission.ID = fmt.Sprintf("perm-%d", atomicAddPermission(&m.permissionSeq))
	permission.SessionID = string(req.SessionID)
	permission.Status = "pending"

	pending := &pendingPermission{
		sessionID:                     job.ID,
		request:                       permission,
		userInputResponseOptionPrefix: userInputResponseOptionPrefix,
		answer:                        make(chan string, 1),
	}
	m.permissionMu.Lock()
	m.pendingPermission[permission.ID] = pending
	m.permissionMu.Unlock()

	m.setJobPermission(job, permission)
	m.publishPermission(job, permission, "permission_request")

	select {
	case optionID := <-pending.answer:
		if optionID == "" {
			return permissionCancelled()
		}
		return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseSelected(acpschema.PermissionOptionID(optionID)))
	case <-ctx.Done():
		m.removePendingPermission(permission.ID)
		m.removeJobPermission(job, permission.ID)
		permission.Status = "cancelled"
		m.publishPermission(job, permission, "permission_response")
		return permissionCancelled()
	}
}

func permissionCancelled() (json.RawMessage, *jsonrpc.Error) {
	return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseCancelled())
}

func (m *Manager) AnswerInteractive(ctx context.Context, req InteractiveAnswer) error {
	text := strings.TrimSpace(req.Text)
	if strings.TrimSpace(req.RequestID) == "" {
		if text == "" {
			return fmt.Errorf("request_id or text is required")
		}
		if job, err := m.job(req.Session); err == nil {
			job.mu.RLock()
			state := job.State
			job.mu.RUnlock()
			if state == StateRunning || state == StateStarting {
				return m.steerText(ctx, job, text, req)
			}
		}
		_, err := m.Send(ctx, SendRequest{
			Session:       req.Session,
			Message:       text,
			Completion:    CompletionAsync,
			PlanRequested: req.PlanRequested,
			ParentVisible: req.ParentVisible,
		})
		return err
	}
	m.permissionMu.Lock()
	pending := m.pendingPermission[req.RequestID]
	if pending == nil {
		m.permissionMu.Unlock()
		return fmt.Errorf("pending permission request not found: %s", req.RequestID)
	}
	job := m.jobByID(pending.sessionID)
	if job == nil {
		m.permissionMu.Unlock()
		return fmt.Errorf("active acp session not found: %s", pending.sessionID)
	}
	if req.Session != "" && req.Session != job.ID && req.Session != job.ParentID {
		m.permissionMu.Unlock()
		return fmt.Errorf("permission request %s does not belong to session %s", req.RequestID, req.Session)
	}
	parentVisible := req.ParentVisible || (req.Session != "" && req.Session == job.ParentID)
	if parentVisible {
		job.mu.Lock()
		job.ParentVisible = true
		job.mu.Unlock()
	}
	if len(req.Answers) > 0 {
		if len(pending.request.Questions) == 0 {
			m.permissionMu.Unlock()
			return fmt.Errorf("permission request %s does not accept structured answers", req.RequestID)
		}
		optionID, err := encodeUserInputResponse(req.Answers, pending.userInputResponseOptionPrefix)
		if err != nil {
			m.permissionMu.Unlock()
			return err
		}
		answerText := formatPermissionAnswers(pending.request, req.Answers)
		delete(m.pendingPermission, req.RequestID)
		m.permissionMu.Unlock()

		resolved := pending.request
		resolved.Status = "selected"
		resolved.SelectedOptionID = "answered"
		m.removeJobPermission(job, req.RequestID)
		m.appendUserAnswerMessage(job, answerText, parentVisible)
		m.publishPermission(job, resolved, "permission_response")

		select {
		case pending.answer <- optionID:
		default:
		}
		return nil
	}
	if strings.TrimSpace(req.OptionID) == "" {
		if text == "" {
			m.permissionMu.Unlock()
			return fmt.Errorf("option_id or text is required")
		}
		delete(m.pendingPermission, req.RequestID)
		m.permissionMu.Unlock()
		cancelled := pending.request
		cancelled.Status = "cancelled"
		m.removeJobPermission(job, req.RequestID)
		m.publishPermission(job, cancelled, "permission_response")
		select {
		case pending.answer <- "":
		default:
		}
		go m.sendTextAfterTurn(job.ID, text, parentVisible, req.PlanRequested)
		return nil
	}
	if _, ok := permissionOption(pending.request.Options, req.OptionID); !ok {
		m.permissionMu.Unlock()
		return fmt.Errorf("unknown permission option: %s", req.OptionID)
	}
	delete(m.pendingPermission, req.RequestID)
	m.permissionMu.Unlock()

	// The agent owns the mode transition out of plan: Claude's ExitPlanMode
	// approval makes the adapter switch modes and emit current_mode_update, and
	// the next non-plan turn re-applies the baseline. Jaz does not second-guess it.
	resolved := pending.request
	resolved.Status = "selected"
	resolved.SelectedOptionID = req.OptionID
	m.removeJobPermission(job, req.RequestID)
	m.publishPermission(job, resolved, "permission_response")

	select {
	case pending.answer <- req.OptionID:
	default:
	}
	if text != "" {
		go m.sendTextAfterTurn(job.ID, text, parentVisible, req.PlanRequested)
	}
	return nil
}

func (m *Manager) steerText(ctx context.Context, job *jobState, text string, req InteractiveAnswer) error {
	if req.ParentVisible {
		job.mu.Lock()
		job.ParentVisible = true
		job.mu.Unlock()
	}
	_, err := m.Steer(ctx, SteerRequest{
		Session:       job.ID,
		Message:       text,
		ParentVisible: req.ParentVisible,
	})
	if err == nil || !errors.Is(err, ErrPromptQueueingUnsupported) {
		return err
	}
	if _, err := m.Cancel(ctx, job.ID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = m.Send(ctx, SendRequest{
		Session:       job.ID,
		Message:       text,
		Completion:    CompletionAsync,
		PlanRequested: req.PlanRequested,
		ParentVisible: req.ParentVisible,
	})
	return err
}

func (m *Manager) sendTextAfterTurn(sessionID, text string, parentVisible, planRequested bool) {
	job := m.jobByID(sessionID)
	if job == nil {
		return
	}
	done := job.turnDone()
	if done != nil {
		<-done
	}
	_, _ = m.Send(context.Background(), SendRequest{
		Session:       sessionID,
		Message:       text,
		Completion:    CompletionAsync,
		PlanRequested: planRequested,
		ParentVisible: parentVisible,
	})
}

func (m *Manager) cancelPendingPermissions(sessionID string) {
	m.permissionMu.Lock()
	pending := make([]*pendingPermission, 0)
	for id, candidate := range m.pendingPermission {
		if candidate.sessionID == sessionID {
			pending = append(pending, candidate)
			delete(m.pendingPermission, id)
		}
	}
	m.permissionMu.Unlock()

	for _, candidate := range pending {
		cancelled := candidate.request
		cancelled.Status = "cancelled"
		if job := m.jobByID(sessionID); job != nil {
			m.removeJobPermission(job, candidate.request.ID)
			m.publishPermission(job, cancelled, "permission_response")
		}
		select {
		case candidate.answer <- "":
		default:
		}
	}
}

func (m *Manager) removePendingPermission(requestID string) {
	m.permissionMu.Lock()
	delete(m.pendingPermission, requestID)
	m.permissionMu.Unlock()
}

func permissionEvent(req acpschema.RequestPermissionRequest) sessionevents.ACPPermission {
	out := sessionevents.ACPPermission{
		Title:      firstNonEmpty(req.ToolCall.Title, "Permission requested"),
		ToolCallID: string(req.ToolCall.ToolCallID),
		Content:    permissionPlanContent(req.ToolCall),
		Options:    make([]sessionevents.ACPPermissionOption, 0, len(req.Options)),
		Locations:  make([]sessionevents.ACPPermissionLocation, 0, len(req.ToolCall.Locations)),
	}
	for _, option := range req.Options {
		out.Options = append(out.Options, sessionevents.ACPPermissionOption{
			ID:   string(option.OptionID),
			Name: option.Name,
			Kind: string(option.Kind),
		})
	}
	for _, location := range req.ToolCall.Locations {
		out.Locations = append(out.Locations, sessionevents.ACPPermissionLocation{
			Path: location.Path,
			Line: location.Line,
		})
	}
	if questions := codexUserInputQuestions(req); len(questions) > 0 {
		out.Title = firstNonEmpty(req.ToolCall.Title, "Clarifying questions")
		out.Questions = questions
	}
	return out
}

// permissionPlanContent returns the proposed-plan markdown carried by a plan-exit
// (switch_mode) permission. Claude's ExitPlanMode sends the plan both as
// rawInput {"plan": ...} and as a text content block; either is the full plan the
// user is being asked to approve, so the approval surface can render it.
func permissionPlanContent(call acpschema.ToolCallUpdate) string {
	if kindString(call.Kind) != string(acpschema.ToolKindSwitchMode) {
		return ""
	}
	var in struct {
		Plan string `json:"plan"`
	}
	if len(call.RawInput) > 0 && json.Unmarshal(call.RawInput, &in) == nil {
		if plan := strings.TrimSpace(in.Plan); plan != "" {
			return clampToolText(plan)
		}
	}
	var b strings.Builder
	for _, block := range normalizeToolContent(call.Content) {
		if block.Type != "text" || block.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(block.Text)
	}
	return clampToolText(strings.TrimSpace(b.String()))
}

type codexUserInputMeta struct {
	CallID    string                   `json:"call_id"`
	TurnID    string                   `json:"turn_id"`
	Questions []codexUserInputQuestion `json:"questions"`
}

type codexUserInputQuestion struct {
	ID       string                 `json:"id"`
	Header   string                 `json:"header"`
	Question string                 `json:"question"`
	IsOther  bool                   `json:"isOther"`
	IsSecret bool                   `json:"isSecret"`
	Options  []codexUserInputOption `json:"options"`
}

type codexUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func codexUserInputQuestions(req acpschema.RequestPermissionRequest) []sessionevents.ACPQuestion {
	var meta codexUserInputMeta
	if !decodeCodexUserInputMeta(req, &meta) {
		return nil
	}
	out := make([]sessionevents.ACPQuestion, 0, len(meta.Questions))
	for _, question := range meta.Questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Question) == "" {
			continue
		}
		options := make([]sessionevents.ACPQuestionOption, 0, len(question.Options))
		for _, option := range question.Options {
			if strings.TrimSpace(option.Label) == "" {
				continue
			}
			options = append(options, sessionevents.ACPQuestionOption{
				Label:       option.Label,
				Description: option.Description,
			})
		}
		out = append(out, sessionevents.ACPQuestion{
			ID:       question.ID,
			Header:   question.Header,
			Question: question.Question,
			IsOther:  question.IsOther,
			IsSecret: question.IsSecret,
			Options:  options,
		})
	}
	return out
}

func decodeCodexUserInputMeta(req acpschema.RequestPermissionRequest, out *codexUserInputMeta) bool {
	return decodeMeta(req.ToolCall.Meta, codexRequestUserInputMetaKey, out) ||
		decodeMeta(req.Meta, codexRequestUserInputMetaKey, out)
}

func decodeMeta(meta map[string]any, key string, out any) bool {
	if len(meta) == 0 {
		return false
	}
	value, ok := meta[key]
	if !ok {
		return false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func permissionOption(options []sessionevents.ACPPermissionOption, optionID string) (sessionevents.ACPPermissionOption, bool) {
	for _, option := range options {
		if option.ID == optionID {
			return option, true
		}
	}
	return sessionevents.ACPPermissionOption{}, false
}

func encodeUserInputResponse(answers map[string]InteractiveAnswerValue, optionPrefix string) (string, error) {
	if optionPrefix == "" {
		optionPrefix = userInputResponseOptionPrefix
	}
	raw, err := json.Marshal(map[string]any{"answers": answers})
	if err != nil {
		return "", err
	}
	return optionPrefix + string(raw), nil
}

func formatPermissionAnswers(permission sessionevents.ACPPermission, answers map[string]InteractiveAnswerValue) string {
	if len(answers) == 0 {
		return ""
	}
	questionByID := make(map[string]sessionevents.ACPQuestion, len(permission.Questions))
	for _, question := range permission.Questions {
		questionByID[question.ID] = question
	}
	ids := make([]string, 0, len(answers))
	for id := range answers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out strings.Builder
	out.WriteString("Answers:")
	for _, id := range ids {
		values := trimmedAnswers(answers[id].Answers)
		if len(values) == 0 {
			continue
		}
		label := id
		if question, ok := questionByID[id]; ok {
			label = firstNonEmpty(question.Header, question.Question, id)
		}
		out.WriteString("\n- ")
		out.WriteString(label)
		out.WriteString(": ")
		out.WriteString(strings.Join(values, ", "))
	}
	if out.String() == "Answers:" {
		return ""
	}
	return out.String()
}

func trimmedAnswers(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (m *Manager) appendUserAnswerMessage(job *jobState, text string, parentVisible bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	sessionIDs := []string{job.ID}
	if parentVisible && job.ParentID != "" && job.ParentID != job.ID {
		sessionIDs = append(sessionIDs, job.ParentID)
	}
	for _, sessionID := range sessionIDs {
		_ = m.store.AppendMessages(sessionID, provider.UserMessage(text))
	}
	m.touchAttention(sessionIDs...)
}

func (m *Manager) setJobPermission(job *jobState, permission sessionevents.ACPPermission) {
	job.mu.Lock()
	defer job.mu.Unlock()
	now := time.Now().UTC()
	for i, candidate := range job.Permissions {
		if candidate.ID == permission.ID {
			job.Permissions[i] = permission
			job.UpdatedAt = now
			job.LastEventAt = now
			return
		}
	}
	job.Permissions = append(job.Permissions, permission)
	job.UpdatedAt = now
	job.LastEventAt = now
}

func (m *Manager) removeJobPermission(job *jobState, requestID string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	now := time.Now().UTC()
	for i, permission := range job.Permissions {
		if permission.ID == requestID {
			job.Permissions = append(job.Permissions[:i], job.Permissions[i+1:]...)
			job.UpdatedAt = now
			job.LastEventAt = now
			return
		}
	}
}

func (m *Manager) publishPermission(job *jobState, permission sessionevents.ACPPermission, eventType string) {
	snapshot := job.Snapshot()
	if eventType == "permission_request" {
		m.touchJobAttention(job)
	}
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&snapshot)))
	for _, sessionID := range surfaceSessionIDs(&snapshot) {
		events = append(events, sessionevents.Event{
			SessionID:  sessionID,
			Type:       eventType,
			Permission: &permission,
			At:         time.Now().UTC(),
		})
	}
	m.publishACPStateAndEvents(snapshot, events...)
}

func (m *Manager) touchJobAttention(job *jobState) {
	snapshot := job.Snapshot()
	m.touchAttention(surfaceSessionIDs(&snapshot)...)
}

func (m *Manager) touchAttention(sessionIDs ...string) {
	seen := map[string]bool{}
	for _, sessionID := range sessionIDs {
		if sessionID == "" || seen[sessionID] {
			continue
		}
		seen[sessionID] = true
		_ = m.store.TouchSessionAttention(sessionID)
	}
}

func (m *Manager) publishSessionChanged(sessionID string) {
	if m.Events == nil || sessionID == "" {
		return
	}
	m.Events.Publish(sessionevents.Event{SessionID: sessionID, Type: sessionevents.TypeSession})
}

func (m *Manager) publishACP(job Job) {
	acp := EventFromJob(job)
	events := make([]sessionevents.Event, 0, 2)
	for _, sessionID := range childSessionIDs(&job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	parentACP := parentSurfaceACP(acp)
	for _, sessionID := range parentSessionIDs(&job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       parentACP,
			At:        time.Now().UTC(),
		})
	}
	m.publishACPStateAndEvents(job, events...)
}

func (m *Manager) publishACPStatus(job Job) {
	acp := EventFromJob(job)
	acp.Assistant = ""
	acp.Thought = ""
	acp.Plan = nil
	acp.ToolCalls = nil
	acp.Permissions = nil
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&job)))
	for _, sessionID := range surfaceSessionIDs(&job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	m.publishACPStateAndEvents(job, events...)
}

func (m *Manager) publishACPMessage(job Job, content string) {
	m.publishACPTranscriptEvent(job, "acp_message", content, nil)
}

func (m *Manager) publishACPThought(job Job, content string) {
	m.publishACPTranscriptEvent(job, "acp_thought", "", func(event *sessionevents.ACPEvent) {
		event.Thought = content
	})
}

func (m *Manager) publishACPTool(job Job, call sessionevents.ACPToolCall) {
	m.publishACPTranscriptEvent(job, "acp_tool", "", func(event *sessionevents.ACPEvent) {
		event.ToolCalls = CloneToolCalls([]sessionevents.ACPToolCall{call})
	})
}

func (m *Manager) publishProviderSubagent(job Job, subagent sessionevents.ProviderSubagentEvent) {
	if subagent.ID == "" {
		return
	}
	if subagent.Provider == "" {
		subagent.Provider = CanonicalAgentName(job.ACPAgent)
	}
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&job)))
	for _, sessionID := range surfaceSessionIDs(&job) {
		events = append(events, sessionevents.Event{
			SessionID:        sessionID,
			Type:             sessionevents.TypeProviderSubagent,
			ProviderSubagent: &subagent,
			At:               time.Now().UTC(),
		})
	}
	m.publishOrderedACPEvents(job, events...)
}

func (m *Manager) publishACPTranscriptEvent(job Job, eventType, content string, customize func(*sessionevents.ACPEvent)) {
	acp := acpTranscriptEnvelope(job)
	if customize != nil {
		customize(acp)
	}
	events := make([]sessionevents.Event, 0, len(childSessionIDs(&job)))
	for _, sessionID := range childSessionIDs(&job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      eventType,
			Content:   content,
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	m.publishACPStateAndEvents(job, events...)
}

func (m *Manager) publishACPStateAndEvents(job Job, events ...sessionevents.Event) {
	m.withACPTranscriptBarrier(job, func() {
		m.saveACPState(job)
		m.recordAndPublishEventListDirect(events)
	})
}

func (m *Manager) publishOrderedACPEvents(job Job, events ...sessionevents.Event) {
	m.withACPTranscriptBarrier(job, func() {
		m.recordAndPublishEventListDirect(events)
	})
}

func (m *Manager) recordAndPublishDirect(event sessionevents.Event) {
	m.recordAndPublishEventListDirect([]sessionevents.Event{event})
}

func (m *Manager) recordAndPublishEventListDirect(events []sessionevents.Event) {
	for len(events) > 0 {
		sessionID := events[0].SessionID
		n := 1
		for n < len(events) && events[n].SessionID == sessionID {
			n++
		}
		m.recordAndPublishEventsDirect(sessionID, events[:n])
		events = events[n:]
	}
}

func (m *Manager) recordAndPublishEventsDirect(sessionID string, events []sessionevents.Event) {
	if len(events) == 0 {
		return
	}
	now := time.Now().UTC()
	storedEvents := make([]sessionevents.Event, len(events))
	for i := range events {
		if events[i].At.IsZero() {
			events[i].At = now
		}
		stored := events[i]
		stored.ACP = events[i].ACP.SlimForStorage()
		storedEvents[i] = stored
	}
	if sessionID != "" {
		_ = m.store.AppendSessionEvents(sessionID, storedEvents...)
	}
	if m.Events == nil {
		return
	}
	for i := range events {
		events[i].Seq = storedEvents[i].Seq
		m.Events.Publish(events[i])
	}
}

type acpStateSaver interface {
	SaveACPState(string, storage.ACPState) error
}

type acpStateLoader interface {
	LoadACPState(string) (storage.ACPState, error)
}

func (m *Manager) saveACPState(job Job) {
	store, ok := m.store.(acpStateSaver)
	if !ok {
		return
	}
	_ = store.SaveACPState(job.ID, acpStorageState(job))
}

func acpStorageState(job Job) storage.ACPState {
	return storage.ACPState{
		ID:              job.ID,
		Slug:            job.Slug,
		Title:           job.Title,
		ParentID:        job.ParentID,
		ACPAgent:        job.ACPAgent,
		ACPSession:      job.ACPSession,
		Cwd:             job.Cwd,
		ModelProvider:   job.ModelProvider,
		Model:           job.Model,
		ReasoningEffort: job.ReasoningEffort,
		State:           job.State,
		StopReason:      job.StopReason,
		Assistant:       job.Assistant,
		Thought:         job.Thought,
		Plan:            clonePlanEntries(job.Plan),
		ToolCalls:       CloneToolCalls(job.ToolCalls),
		Modes:           acpModeEvent(job.Modes),
		Error:           job.Error,
		ActiveOperation: job.ActiveOperation,
		ParentVisible:   job.ParentVisible,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func EventFromJob(job Job) *sessionevents.ACPEvent {
	return &sessionevents.ACPEvent{
		ID:              job.ID,
		Slug:            job.Slug,
		Title:           job.Title,
		ParentID:        job.ParentID,
		Agent:           job.ACPAgent,
		SessionID:       job.ACPSession,
		ModelProvider:   job.ModelProvider,
		Model:           job.Model,
		ReasoningEffort: job.ReasoningEffort,
		State:           job.State,
		StopReason:      job.StopReason,
		Assistant:       job.Assistant,
		Thought:         job.Thought,
		Error:           job.Error,
		Modes:           acpModeEvent(job.Modes),
		Plan:            clonePlanEntries(job.Plan),
		ToolCalls:       CloneToolCalls(job.ToolCalls),
		Permissions:     clonePermissions(job.Permissions),
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func acpTranscriptEnvelope(job Job) *sessionevents.ACPEvent {
	return &sessionevents.ACPEvent{
		ID:              job.ID,
		Slug:            job.Slug,
		Title:           job.Title,
		ParentID:        job.ParentID,
		Agent:           job.ACPAgent,
		SessionID:       job.ACPSession,
		ModelProvider:   job.ModelProvider,
		Model:           job.Model,
		ReasoningEffort: job.ReasoningEffort,
		State:           job.State,
		StopReason:      job.StopReason,
		Error:           job.Error,
		Modes:           acpModeEvent(job.Modes),
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func acpModeEvent(modes ModeState) sessionevents.ACPModeState {
	out := sessionevents.ACPModeState{
		CurrentModeID:  modes.CurrentModeID,
		PlanModeID:     modes.PlanModeID,
		AvailableModes: make([]sessionevents.ACPMode, 0, len(modes.AvailableModes)),
	}
	for _, mode := range modes.AvailableModes {
		out.AvailableModes = append(out.AvailableModes, sessionevents.ACPMode{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return out
}

func childSessionIDs(job *Job) []string {
	if job == nil {
		return nil
	}
	return []string{job.ID}
}

func parentSessionIDs(job *Job) []string {
	if job == nil || job.ParentID == "" || job.ParentID == job.ID || !job.ParentVisible {
		return nil
	}
	return []string{job.ParentID}
}

func surfaceSessionIDs(job *Job) []string {
	return append(childSessionIDs(job), parentSessionIDs(job)...)
}

func parentSurfaceACP(acp *sessionevents.ACPEvent) *sessionevents.ACPEvent {
	if acp == nil {
		return nil
	}
	out := *acp
	out.Assistant = ""
	out.Thought = ""
	out.ToolCalls = nil
	out.Permissions = nil
	return &out
}

func atomicAddPermission(seq *uint64) uint64 {
	return atomic.AddUint64(seq, 1)
}
