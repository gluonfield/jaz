package acp

import (
	"context"
	"fmt"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// Cancel asks the agent to stop the current turn and tears down an agent that
// ignores the protocol cancellation deadline.
func (m *Manager) Cancel(ctx context.Context, ref string) (Job, error) {
	job, err := m.job(ref)
	if err != nil {
		return m.cancelStored(ref)
	}
	running, done := job.requestCancel()
	clearGoal := m.clearGoalOnCancel(job)
	m.log.Info("acp cancel requested", "session", job.ID, "agent", job.ACPAgent, "running", running)
	if peer := m.peer(job.ID); peer != nil {
		if err := peer.Notify(ctx, acpschema.AgentMethodSessionCancel, acpschema.CancelNotification{
			SessionID: acpschema.SessionID(job.ACPSession),
		}); err != nil {
			m.log.Warn("acp cancel notify failed", "session", job.ID, "error", err)
		}
	} else if cancel := job.turnCancel(); cancel != nil {
		cancel()
	}
	if !running || done == nil {
		if clearGoal {
			m.publishGoalClear(job)
		}
		return job.Snapshot(), nil
	}
	select {
	case <-done:
		m.log.Info("acp turn cancelled", "session", job.ID)
	case <-time.After(5 * time.Second):
		m.log.Warn("acp agent ignored cancel, tearing down process", "session", job.ID)
		m.teardown(job.ID)
		job.mu.RLock()
		stillRunning := job.State == StateRunning || job.State == StateStarting
		job.mu.RUnlock()
		if stillRunning {
			job.setState(StateCancelled, StopReasonCancelled, "")
			m.publishACPStatus(job.eventView())
		}
	case <-ctx.Done():
	}
	if clearGoal {
		m.publishGoalClear(job)
	}
	return job.Snapshot(), nil
}

func (m *Manager) clearGoalOnCancel(job *jobState) bool {
	job.mu.RLock()
	turnRequested := job.turn != nil && job.turn.goalRequested
	job.mu.RUnlock()
	if turnRequested {
		return true
	}
	session, err := m.store.LoadSession(job.ID)
	return err == nil && goal.Active(session.Goal)
}

func (m *Manager) cancelStored(ref string) (Job, error) {
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return Job{}, fmt.Errorf("session not found: %s", ref)
	}
	m.log.Info("cancel for inactive session", "session", session.ID)
	now := time.Now().UTC()
	session.Status = storage.StatusIdle
	session.Error = ""
	session.UpdatedAt = now
	if err := m.store.SaveSession(session); err != nil {
		return Job{}, err
	}
	agentName, acpSessionID, cwd := "", "", ""
	if session.RuntimeRef != nil {
		agentName = session.RuntimeRef.Agent
		acpSessionID = session.RuntimeRef.SessionID
		cwd = session.RuntimeRef.Cwd
	}
	cancelled := Job{
		ID: session.ID, Slug: session.Slug, Title: session.Title, ParentID: session.ParentID,
		ACPAgent: agentName, ACPSession: acpSessionID, Cwd: cwd,
		ModelProvider: session.ModelProvider, Model: session.Model, ReasoningEffort: session.ReasoningEffort,
		State: StateCancelled, StopReason: StopReasonCancelled,
		CreatedAt: session.CreatedAt, UpdatedAt: now, LastEventAt: now,
	}
	events := []sessionevents.Event{{
		SessionID: session.ID,
		Type:      "acp",
		ACP:       EventFromJob(cancelled),
	}}
	if goal.Active(session.Goal) {
		events = append(events, sessionevents.Event{
			SessionID: session.ID,
			Type:      sessionevents.TypeGoalClear,
			At:        now,
		})
	}
	m.publishOrderedACPEvents(eventViewFromJob(cancelled), events...)
	return cancelled, nil
}
