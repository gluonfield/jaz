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

func (m *Manager) awaitPermission(ctx context.Context, job *Job, req acpschema.RequestPermissionRequest) (json.RawMessage, *jsonrpc.Error) {
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
	if strings.TrimSpace(req.RequestID) == "" {
		text := strings.TrimSpace(req.Text)
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
		if strings.TrimSpace(req.Text) == "" {
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
		go m.sendTextAfterTurn(job.ID, req.Text, parentVisible)
		return nil
	}
	option, ok := permissionOption(pending.request.Options, req.OptionID)
	if !ok {
		m.permissionMu.Unlock()
		return fmt.Errorf("unknown permission option: %s", req.OptionID)
	}
	m.permissionMu.Unlock()
	if optionApprovesPlan(option) {
		if err := m.restoreBaselineMode(ctx, job); err != nil {
			return err
		}
	}

	m.permissionMu.Lock()
	if m.pendingPermission[req.RequestID] != pending {
		m.permissionMu.Unlock()
		return fmt.Errorf("pending permission request not found: %s", req.RequestID)
	}
	delete(m.pendingPermission, req.RequestID)
	m.permissionMu.Unlock()

	resolved := pending.request
	resolved.Status = "selected"
	resolved.SelectedOptionID = req.OptionID
	m.removeJobPermission(job, req.RequestID)
	m.publishPermission(job, resolved, "permission_response")

	select {
	case pending.answer <- req.OptionID:
	default:
	}
	return nil
}

func (m *Manager) steerText(ctx context.Context, job *Job, text string, req InteractiveAnswer) error {
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

func (m *Manager) sendTextAfterTurn(sessionID, text string, parentVisible bool) {
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

func optionApprovesPlan(option sessionevents.ACPPermissionOption) bool {
	text := strings.ToLower(option.ID + " " + option.Name + " " + option.Kind)
	return strings.Contains(text, "approve") || strings.Contains(text, "allow")
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

func (m *Manager) appendUserAnswerMessage(job *Job, text string, parentVisible bool) {
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

func (m *Manager) setJobPermission(job *Job, permission sessionevents.ACPPermission) {
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

func (m *Manager) removeJobPermission(job *Job, requestID string) {
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

func (m *Manager) publishPermission(job *Job, permission sessionevents.ACPPermission, eventType string) {
	m.saveACPState(job.Snapshot())
	if eventType == "permission_request" {
		m.touchJobAttention(job)
	}
	for _, sessionID := range surfaceSessionIDs(job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID:  sessionID,
			Type:       eventType,
			Permission: &permission,
			At:         time.Now().UTC(),
		})
	}
}

func (m *Manager) touchJobAttention(job *Job) {
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
	m.saveACPState(job)
	acp := acpEvent(job)
	for _, sessionID := range childSessionIDs(&job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	parentACP := parentSurfaceACP(acp)
	for _, sessionID := range parentSessionIDs(&job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       parentACP,
			At:        time.Now().UTC(),
		})
	}
}

func (m *Manager) publishACPStatus(job Job) {
	m.saveACPState(job)
	acp := acpEvent(job)
	acp.Assistant = ""
	acp.Thought = ""
	acp.Plan = nil
	acp.ToolCalls = nil
	acp.Permissions = nil
	for _, sessionID := range surfaceSessionIDs(&job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
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
	for _, sessionID := range surfaceSessionIDs(&job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID:        sessionID,
			Type:             sessionevents.TypeProviderSubagent,
			ProviderSubagent: &subagent,
			At:               time.Now().UTC(),
		})
	}
}

func (m *Manager) publishACPTranscriptEvent(job Job, eventType, content string, customize func(*sessionevents.ACPEvent)) {
	m.saveACPState(job)
	acp := acpEvent(job)
	acp.Assistant = ""
	acp.Thought = ""
	acp.Plan = nil
	acp.ToolCalls = nil
	acp.Permissions = nil
	if customize != nil {
		customize(acp)
	}
	for _, sessionID := range childSessionIDs(&job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID: sessionID,
			Type:      eventType,
			Content:   content,
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
}

func (m *Manager) recordAndPublish(event sessionevents.Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	// Store slim; publish full so subscribers can label sessions they
	// haven't fetched yet.
	stored := event
	stored.ACP = event.ACP.SlimForStorage()
	events := []sessionevents.Event{stored}
	if event.SessionID != "" {
		_ = m.store.AppendSessionEvents(event.SessionID, events...)
	}
	// Publish with the stored Seq so clients can dedupe against history.
	event.Seq = events[0].Seq
	if m.Events != nil {
		m.Events.Publish(event)
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
		ParentVisible:   job.ParentVisible,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func acpEvent(job Job) *sessionevents.ACPEvent {
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
