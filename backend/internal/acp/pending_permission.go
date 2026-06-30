package acp

import "github.com/wins/jaz/backend/internal/sessionevents"

type pendingPermission struct {
	sessionID     string
	request       sessionevents.ACPPermission
	encodeAnswers answerEncoder
	answer        chan string
}

type answerEncoder func(map[string]InteractiveAnswerValue) (string, error)

func (m *Manager) registerPendingPermission(job *jobState, pending *pendingPermission) bool {
	m.permissionMu.Lock()
	defer m.permissionMu.Unlock()
	if job.hasQueuedPromptSuccessor() {
		return false
	}
	m.pendingPermission[pending.request.ID] = pending
	return true
}

func (m *Manager) cancelPendingPermissions(sessionID string) {
	m.permissionMu.Lock()
	pending := m.takePendingPermissionsLocked(sessionID)
	m.permissionMu.Unlock()
	m.cancelPendingPermissionList(pending)
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
	m.cancelPendingPermissionList(pending)
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

func (m *Manager) cancelPendingPermissionList(pending []*pendingPermission) {
	for _, candidate := range pending {
		cancelled := candidate.request
		cancelled.Status = "cancelled"
		if job := m.jobByID(candidate.sessionID); job != nil {
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
