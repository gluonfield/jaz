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
	opts := sendOptions{transcript: sendTranscriptHidden}
	if session.Turn != nil {
		if session.Turn.ActiveOperation == ActiveOperationCompact {
			_, err := m.Compact(ctx, CompactRequest{Session: sessionID})
			return err
		}
		req.PlanRequested = session.Turn.PlanRequested
		req.GoalRequested = session.Turn.GoalRequested
		req.ParentVisible = session.Turn.ParentVisible
		if req.ParentVisible && !session.Turn.NotifyParent {
			req.Completion = CompletionInline
		}
		opts.allowSilence = session.Turn.AllowSilence
		opts.output = session.Turn.Output
		opts.parentOutput = session.Turn.ParentOutput
	}
	_, err = m.send(ctx, req, opts)
	return err
}
