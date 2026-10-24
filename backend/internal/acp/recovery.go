package acp

import (
	"context"
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
)

// interruptedTurnContinuation goes on with a turn whose prompt the agent
// already holds, after a restart or a provider failure stopped it.
const interruptedTurnContinuation = "Continue from where you left off."

func (m *Manager) ResumeInterruptedTurn(ctx context.Context, sessionID string) error {
	session, err := m.store.LoadSession(sessionID)
	if err != nil {
		return err
	}
	unsent := session.Turn != nil && session.Turn.Prompt != nil && session.Turn.Prompt.State == ""
	if session.Turn != nil && session.Turn.ActiveOperation != ActiveOperationCompact &&
		session.Turn.Prompt != nil && session.Turn.Prompt.State == storage.PromptSending {
		return storage.ErrPromptUnconfirmed
	}
	if session.RuntimeRef == nil || (session.RuntimeRef.SessionID == "" && !unsent && !m.configuredLocal(session.RuntimeRef.Agent)) {
		return fmt.Errorf("cannot resume interrupted turn: provider session id is missing")
	}
	req := SendRequest{Session: sessionID, Message: interruptedTurnContinuation, Completion: CompletionAsync}
	opts := sendOptions{transcript: sendTranscriptHidden}
	if session.Turn != nil {
		// Messages steered into the turn that its agent never took in go on
		// with the continuation, which redoing a compaction would hold back.
		if session.Turn.ActiveOperation == ActiveOperationCompact && len(session.Turn.Steers) == 0 {
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
		opts.owes = session.Turn.Owes
		req.ParentOutput = session.Turn.ParentOutput
		opts.retries = session.Turn.Retries
		if unsent {
			req.Message = session.Turn.Prompt.Message
			req.Attachments = session.Turn.Prompt.Attachments
		}
		for _, steer := range session.Turn.Steers {
			req.Message += "\n\n" + steer.Message
			req.Attachments = append(req.Attachments, steer.Attachments...)
		}
	}
	_, err = m.send(ctx, req, opts)
	return err
}
