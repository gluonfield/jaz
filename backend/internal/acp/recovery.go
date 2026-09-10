package acp

import (
	"context"
	"fmt"
)

func (m *Manager) ResumeInterruptedTurn(ctx context.Context, sessionID string) error {
	session, err := m.store.LoadSession(sessionID)
	if err != nil {
		return err
	}
	if session.RuntimeRef == nil || (session.RuntimeRef.SessionID == "" && !m.configuredLocal(session.RuntimeRef.Agent)) {
		return fmt.Errorf("cannot resume interrupted turn: provider session id is missing")
	}
	req := SendRequest{Session: sessionID, Message: "Continue from where you left off.", Completion: CompletionAsync}
	if session.Turn != nil {
		if session.Turn.ActiveOperation == ActiveOperationCompact {
			_, err := m.Compact(ctx, CompactRequest{Session: sessionID})
			return err
		}
		req.PlanRequested = session.Turn.PlanRequested
		req.GoalRequested = session.Turn.GoalRequested
	}
	_, err = m.send(ctx, req, sendOptions{transcript: sendTranscriptHidden})
	return err
}
