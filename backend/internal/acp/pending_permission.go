package acp

import (
	"context"
	"slices"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type permissionAnswer struct {
	OptionID string
	Answers  map[string]InteractiveAnswerValue
}

type pendingPermission struct {
	sessionID      string
	request        sessionevents.ACPPermission
	prepareAnswers answerPreparer
	answer         chan permissionAnswer
	published      chan struct{}
}

type answerPreparer func(map[string]InteractiveAnswerValue) (map[string]InteractiveAnswerValue, error)

func (m *Manager) awaitPermissionAnswer(ctx context.Context, job *jobState, permission sessionevents.ACPPermission, prepare answerPreparer) permissionAnswer {
	job.mu.RLock()
	permission.SessionID = job.ID
	if job.turn != nil {
		permission.TurnStartedAt = job.turn.startedAt
	}
	job.mu.RUnlock()
	if reader, ok := m.store.(storage.MessageRecordReader); ok {
		records, err := reader.LoadMessageRecords(job.ID)
		if err == nil {
			var seq int64
			if len(records) > 0 {
				seq = records[len(records)-1].Seq
			}
			permission.AfterMessageSeq = &seq
		} else {
			m.log.Warn("read question conversation boundary", "session", job.ID, "error", err)
		}
	}
	pending := &pendingPermission{
		sessionID: job.ID, request: permission, prepareAnswers: prepare,
		answer: make(chan permissionAnswer, 1), published: make(chan struct{}),
	}
	m.permissionMu.Lock()
	if ctx.Err() != nil || job.hasQueuedPromptSuccessor() {
		m.permissionMu.Unlock()
		return permissionAnswer{}
	}
	m.pendingPermission[permission.ID] = pending
	m.permissionMu.Unlock()
	m.setJobPermission(job, permission)
	m.publishPermission(job, permission, "permission_request")
	close(pending.published)

	select {
	case answer := <-pending.answer:
		return answer
	case <-ctx.Done():
		m.permissionMu.Lock()
		if m.pendingPermission[permission.ID] == nil {
			m.permissionMu.Unlock()
			return <-pending.answer
		}
		delete(m.pendingPermission, permission.ID)
		m.permissionMu.Unlock()
		m.removeJobPermission(job, permission.ID)
		permission.Status = "cancelled"
		permission.Origin = "unknown"
		m.publishPermission(job, permission, "permission_response")
		return permissionAnswer{}
	}
}

// AwaitsAnswer reports whether a question or plan decision held open right
// now shows in sessionID's thread, its own or one a child thread surfaces
// there. Only these wait on the person: ordinary tool permissions are granted
// as they arrive, and a restart drops what was pending.
func (m *Manager) AwaitsAnswer(sessionID string) bool {
	m.permissionMu.Lock()
	asking := make([]string, 0, len(m.pendingPermission))
	for _, pending := range m.pendingPermission {
		asking = append(asking, pending.sessionID)
	}
	m.permissionMu.Unlock()
	for _, id := range asking {
		if job := m.jobByID(id); job != nil && slices.Contains(surfaceSessionIDs(job.eventView()), sessionID) {
			return true
		}
	}
	return false
}

func (m *Manager) cancelPendingPermissions(sessionID, origin string) {
	m.permissionMu.Lock()
	pending := m.takePendingPermissionsLocked(sessionID)
	m.permissionMu.Unlock()
	m.cancelPendingPermissionList(pending, origin)
}

func (m *Manager) cancelPendingPermissionsForSteer(job *jobState, done chan struct{}) <-chan struct{} {
	m.permissionMu.Lock()
	pending := m.takePendingPermissionsLocked(job.ID)
	var handoff <-chan struct{}
	if len(pending) > 0 {
		handoff = job.requirePromptHandoff(done)
	} else {
		handoff = job.currentPromptHandoff(done)
	}
	m.permissionMu.Unlock()
	m.cancelPendingPermissionList(pending, "user_input")
	return handoff
}

func (m *Manager) takePendingPermissionsLocked(sessionID string) []*pendingPermission {
	pending := make([]*pendingPermission, 0)
	for id, candidate := range m.pendingPermission {
		if candidate.sessionID == sessionID {
			pending = append(pending, candidate)
			delete(m.pendingPermission, id)
		}
	}
	return pending
}

func (m *Manager) cancelPendingPermissionList(pending []*pendingPermission, origin string) {
	for _, candidate := range pending {
		<-candidate.published
		cancelled := candidate.request
		cancelled.Status = "cancelled"
		cancelled.Origin = origin
		if job := m.jobByID(candidate.sessionID); job != nil {
			m.removeJobPermission(job, candidate.request.ID)
			m.publishPermission(job, cancelled, "permission_response")
		}
		candidate.answer <- permissionAnswer{}
	}
}
