package acp

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (m *Manager) Wait(ctx context.Context, req WaitRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		return Job{}, err
	}
	release := m.retainTurnResult(job.ID)
	defer release()
	done := job.turnDone()
	if done == nil {
		return m.turnResult(ctx, job)
	}
	if req.Timeout <= 0 {
		req.Timeout = 10 * time.Minute
	}
	timer := time.NewTimer(req.Timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Job{}, ctx.Err()
	case <-timer.C:
		return job.Snapshot(), nil
	case <-done:
		return m.turnResult(ctx, job)
	}
}

func (m *Manager) turnResult(ctx context.Context, job *jobState) (Job, error) {
	snapshot := job.Snapshot()
	if !job.resultDiscarded() {
		return snapshot, nil
	}
	return m.restoreTurnResult(ctx, snapshot)
}

func (m *Manager) RetainStream(sessionID string) func() {
	return m.retainTurnResult(sessionID)
}

func (m *Manager) retainTurnResult(sessionID string) func() {
	m.mu.Lock()
	if m.turnReaders == nil {
		m.turnReaders = make(map[string]int)
	}
	m.turnReaders[sessionID]++
	m.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() { m.releaseTurnResult(sessionID) })
	}
}

func (m *Manager) releaseTurnResult(sessionID string) {
	var discard *jobState
	m.mu.Lock()
	if readers := m.turnReaders[sessionID]; readers > 1 {
		m.turnReaders[sessionID] = readers - 1
	} else {
		delete(m.turnReaders, sessionID)
		discard = m.pendingDiscard[sessionID]
		delete(m.pendingDiscard, sessionID)
	}
	m.mu.Unlock()
	if discard != nil {
		discard.discardTurnResult()
	}
}

func (m *Manager) discardTurnResultWhenReleased(job *jobState) {
	m.mu.Lock()
	if m.turnReaders[job.ID] > 0 {
		if m.pendingDiscard == nil {
			m.pendingDiscard = make(map[string]*jobState)
		}
		m.pendingDiscard[job.ID] = job
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	job.discardTurnResult()
}

func (m *Manager) restoreTurnResult(ctx context.Context, job Job) (Job, error) {
	events, err := m.store.LoadLatestACPTurn(ctx, job.ID)
	if err != nil {
		return Job{}, err
	}
	tools := make(map[string]sessionevents.ACPToolCall)
	permissions := make(map[string]sessionevents.ACPPermission)
	var assistant, thought strings.Builder
	for _, event := range events {
		if event.Type == "acp" && event.ACP != nil && event.ACP.ID == job.ID {
			if event.ACP.State != "" {
				job.State = event.ACP.State
			}
			job.StopReason = event.ACP.StopReason
			job.Error = event.ACP.Error
			if event.ACP.Plan != nil {
				job.Plan = clonePlanEntries(event.ACP.Plan)
			}
		}
		switch event.Type {
		case sessionevents.TypeACPMessage:
			if event.ACP != nil && event.ACP.ID == job.ID {
				assistant.WriteString(event.Content)
			}
		case sessionevents.TypeACPThought:
			if event.ACP != nil && event.ACP.ID == job.ID {
				thought.WriteString(event.ACP.Thought)
			}
		case "acp_tool":
			if event.ACP == nil || event.ACP.ID != job.ID {
				continue
			}
			for _, call := range event.ACP.ToolCalls {
				tools[call.ID] = call
			}
		case "plan_update", "proposed_plan":
			if event.Plan != nil {
				job.Plan = clonePlanEntries(event.Plan.Plan)
			}
		case "permission_request":
			if event.Permission != nil {
				permissions[event.Permission.ID] = *event.Permission
			}
		case "permission_response":
			if event.Permission != nil {
				delete(permissions, event.Permission.ID)
			}
		}
	}
	job.Assistant = assistant.String()
	job.Thought = thought.String()
	job.ToolCalls = sortedToolCalls(tools)
	job.Permissions = sortedPermissions(permissions)
	return job, nil
}

func sortedPermissions(in map[string]sessionevents.ACPPermission) []sessionevents.ACPPermission {
	if len(in) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPPermission, 0, len(in))
	for _, permission := range in {
		out = append(out, permission)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
