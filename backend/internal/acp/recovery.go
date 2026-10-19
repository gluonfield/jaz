package acp

import (
	"context"
	"fmt"
)

// interruptedTurnContinuation goes on with a turn whose prompt the agent
// already holds, after a restart or a provider failure stopped it.
const interruptedTurnContinuation = "Continue from where you left off."

func (m *Manager) ResumeInterruptedTurn(ctx context.Context, sessionID string) error {
	session, err := m.store.LoadSession(sessionID)
	if err != nil {
		return err
	}
	unsent := session.Turn != nil && session.Turn.Prompt != nil
	if session.RuntimeRef == nil || (session.RuntimeRef.SessionID == "" && !unsent && !m.configuredLocal(session.RuntimeRef.Agent)) {
		return fmt.Errorf("cannot resume interrupted turn: provider session id is missing")
	}
	req := SendRequest{Session: sessionID, Message: interruptedTurnContinuation, Completion: CompletionAsync}
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
		req.ParentOutput = session.Turn.ParentOutput
		opts.retries = session.Turn.Retries
		if unsent {
			req.Message = session.Turn.Prompt.Message
			req.Attachments = session.Turn.Prompt.Attachments
		}
	}
	_, err = m.send(ctx, req, opts)
	return err
}
