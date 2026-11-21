package acp

import (
	"context"
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) SetDeferredMCPTools(ctx context.Context, ref string, deferred bool) error {
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return err
	}
	if job := m.jobByID(session.ID); job != nil {
		if done := job.finishingTurn(); done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	defer m.resumeLocks.Lock(session.ID)()
	session, err = m.store.LoadSession(session.ID)
	if err != nil {
		return err
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return fmt.Errorf("session %s is not acp-backed", session.ID)
	}
	if m.configuredLocal(session.RuntimeRef.Agent) {
		return fmt.Errorf("agent %q does not use an external MCP client", session.RuntimeRef.Agent)
	}
	if session.RuntimeRef.DeferredMCPTools == deferred {
		return nil
	}
	job := m.jobByID(session.ID)
	if job != nil && (m.subagentsRunning(job) || m.hasLivePermissionWaiter(job.ID) || !job.retire()) {
		return fmt.Errorf("wait for the agent to become idle before changing MCP discovery")
	}
	session.RuntimeRef.DeferredMCPTools = deferred
	if err := m.store.SaveSession(session); err != nil {
		if job != nil {
			job.mu.Lock()
			job.retired = false
			job.mu.Unlock()
		}
		return err
	}
	m.teardown(session.ID)
	m.publishSessionChanged(session.ID)
	return nil
}
