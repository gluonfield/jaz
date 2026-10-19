package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/google/uuid"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type InteractiveAnswerValue struct {
	Answers []string `json:"answers"`
}

func (m *Manager) awaitPermission(ctx context.Context, job *jobState, req acpschema.RequestPermissionRequest) (json.RawMessage, *jsonrpc.Error) {
	if ctx.Err() != nil || job.hasQueuedPromptSuccessor() {
		return permissionCancelled()
	}
	if job.permissionToolCall(req.ToolCall).Kind != string(acpschema.ToolKindSwitchMode) {
		for _, kind := range []acpschema.PermissionOptionKind{acpschema.PermissionOptionKindAllowOnce, acpschema.PermissionOptionKindAllowAlways} {
			for _, option := range req.Options {
				if option.Kind == kind {
					return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseSelected(option.OptionID))
				}
			}
		}
		return permissionCancelled()
	}
	permission := job.permissionEvent(req)
	permission.ID = newPermissionID()
	permission.SessionID = string(req.SessionID)
	permission.Status = "pending"

	answer := m.awaitPermissionAnswer(ctx, job, permission, nil)
	if answer.OptionID == "" {
		return permissionCancelled()
	}
	return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseSelected(acpschema.PermissionOptionID(answer.OptionID)))
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
			FromUser:      true,
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
		if job.turn != nil {
			if err := m.store.SetTurnIntent(job.ID, job.turn.goalRequested, true, job.turn.completion.propagates() && !job.turn.planRequested); err != nil {
				job.mu.Unlock()
				m.permissionMu.Unlock()
				return err
			}
		}
		job.ParentVisible = true
		job.mu.Unlock()
	}
	if req.Answers != nil {
		if len(pending.request.Questions) == 0 {
			m.permissionMu.Unlock()
			return fmt.Errorf("permission request %s does not accept structured answers", req.RequestID)
		}
		answers := req.Answers
		if pending.prepareAnswers != nil {
			var err error
			answers, err = pending.prepareAnswers(answers)
			if err != nil {
				m.permissionMu.Unlock()
				return err
			}
		}
		delete(m.pendingPermission, req.RequestID)
		m.permissionMu.Unlock()
		<-pending.published

		resolved := pending.request
		resolved.Status = "selected"
		resolved.SelectedOptionID = "answered"
		resolved.Answers = make(map[string][]string, len(resolved.Questions))
		for _, question := range resolved.Questions {
			resolved.Answers[question.ID] = trimmedAnswers(answers[question.ID].Answers)
		}
		m.removeJobPermission(job, req.RequestID)
		m.publishPermission(job, resolved, "permission_response")

		pending.answer <- permissionAnswer{Answers: answers}
		return nil
	}
	if strings.TrimSpace(req.OptionID) == "" {
		if text == "" {
			m.permissionMu.Unlock()
			return fmt.Errorf("option_id or text is required")
		}
		delete(m.pendingPermission, req.RequestID)
		m.permissionMu.Unlock()
		<-pending.published
		cancelled := pending.request
		cancelled.Status = "cancelled"
		m.removeJobPermission(job, req.RequestID)
		m.publishPermission(job, cancelled, "permission_response")
		pending.answer <- permissionAnswer{}
		go m.sendTextAfterTurn(job.ID, text, parentVisible, req.PlanRequested)
		return nil
	}
	if _, ok := permissionOption(pending.request.Options, req.OptionID); !ok {
		m.permissionMu.Unlock()
		return fmt.Errorf("unknown permission option: %s", req.OptionID)
	}
	delete(m.pendingPermission, req.RequestID)
	m.permissionMu.Unlock()
	<-pending.published

	// The agent owns the mode transition out of plan: Claude's ExitPlanMode
	// approval makes the adapter switch modes and emit current_mode_update, and
	// the next non-plan turn re-applies the baseline. Jaz does not second-guess it.
	resolved := pending.request
	resolved.Status = "selected"
	resolved.SelectedOptionID = req.OptionID
	m.removeJobPermission(job, req.RequestID)
	m.publishPermission(job, resolved, "permission_response")

	pending.answer <- permissionAnswer{OptionID: req.OptionID}
	if text != "" {
		go m.sendTextAfterTurn(job.ID, text, parentVisible, req.PlanRequested)
	}
	return nil
}

func (m *Manager) steerText(ctx context.Context, job *jobState, text string, req InteractiveAnswer) error {
	_, err := m.Steer(ctx, SteerRequest{
		Session:       job.ID,
		Message:       text,
		FromUser:      true,
		ParentVisible: req.ParentVisible,
	})
	if err == nil || !errors.Is(err, ErrSteeringUnsupported) {
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
		FromUser:      true,
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
		FromUser:      true,
		Completion:    CompletionAsync,
		PlanRequested: planRequested,
		ParentVisible: parentVisible,
	})
}

// permissionToolCall overlays the request's tool call on the call it updates: an
// agent can leave out the fields that did not change, such as the kind, title
// and locations.
func (j *jobState) permissionToolCall(update acpschema.ToolCallUpdate) sessionevents.ACPToolCall {
	j.mu.RLock()
	call := j.toolByID[string(update.ToolCallID)]
	j.mu.RUnlock()
	mergeToolCall(&call, toolUpdateSnapshot(update))
	return call
}

func (j *jobState) permissionEvent(req acpschema.RequestPermissionRequest) sessionevents.ACPPermission {
	call := j.permissionToolCall(req.ToolCall)
	out := sessionevents.ACPPermission{
		Title:      firstNonEmpty(call.Title, "Permission requested"),
		ToolCallID: string(req.ToolCall.ToolCallID),
		Content:    permissionPlanContent(call.Kind, req.ToolCall.RawInput),
		Options:    make([]sessionevents.ACPPermissionOption, 0, len(req.Options)),
		Locations:  make([]sessionevents.ACPPermissionLocation, 0, len(call.Locations)),
	}
	for _, option := range req.Options {
		out.Options = append(out.Options, sessionevents.ACPPermissionOption{
			ID:   string(option.OptionID),
			Name: option.Name,
			Kind: string(option.Kind),
		})
	}
	for _, location := range call.Locations {
		out.Locations = append(out.Locations, sessionevents.ACPPermissionLocation{
			Path: location.Path,
			Line: location.Line,
		})
	}
	return out
}

// permissionPlanContent returns the plan a plan-exit (switch_mode) permission
// asks the user to approve; Claude and Codex both send it as rawInput.plan.
func permissionPlanContent(kind string, rawInput json.RawMessage) string {
	var in struct {
		Plan string `json:"plan"`
	}
	if kind != string(acpschema.ToolKindSwitchMode) || json.Unmarshal(rawInput, &in) != nil {
		return ""
	}
	return clampToolText(strings.TrimSpace(in.Plan))
}

func permissionOption(options []sessionevents.ACPPermissionOption, optionID string) (sessionevents.ACPPermissionOption, bool) {
	for _, option := range options {
		if option.ID == optionID {
			return option, true
		}
	}
	return sessionevents.ACPPermissionOption{}, false
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
	snapshot := job.eventView()
	if eventType == "permission_request" {
		m.touchJobAttention(job)
	}
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(snapshot)))
	for _, sessionID := range surfaceSessionIDs(snapshot) {
		events = append(events, sessionevents.Event{
			SessionID:  sessionID,
			Type:       eventType,
			Permission: &permission,
			At:         time.Now().UTC(),
		})
	}
	m.publishOrderedACPEvents(snapshot, events...)
}

func newPermissionID() string {
	return "perm-" + uuid.NewString()
}
