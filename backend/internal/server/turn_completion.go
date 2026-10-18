package server

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Server) HandleACPTurnFinished(_ context.Context, job acp.Job) {
	if err := s.finishTurn(job); err != nil {
		s.logger().Error("finish turn and deliver result", "session", job.ID, "error", err)
		_ = s.Store.UpdateSessionStatus(job.ID, storage.StatusInterrupted, err.Error(), time.Time{})
		s.publishSessionChanged(job.ID)
	}
}

// finishTurn does the bookkeeping of a turn that ended: it delivers the
// turn's result to whoever waits for it, closes its routine run and records
// how it ended.
func (s *Server) finishTurn(job acp.Job) error {
	status := storage.SessionStatusForACPState(job.State, job.StopReason)
	if job.ID == "" || status == "" {
		return nil
	}
	session, err := s.Store.LoadSession(job.ID)
	if err != nil {
		return err
	}
	ids := []string{job.ID}
	if session.ParentID != "" {
		ids = append(ids, session.ParentID)
	}
	if session.Turn != nil && session.Turn.Output != nil && session.Turn.Output.ReplyTo != "" {
		ids = append(ids, session.Turn.Output.ReplyTo)
	}
	unlock := s.lockSession(ids...)
	session, err = s.Store.LoadSession(job.ID)
	var deliveries []storage.TurnDelivery
	if err == nil && session.Turn != nil && status != storage.StatusInterrupted {
		if session.Turn.NotifyParent && session.ParentID != "" {
			parent, loadErr := s.Store.LoadSession(session.ParentID)
			if loadErr != nil {
				err = loadErr
			} else if parent.RuntimeRef != nil && parent.RuntimeRef.Agent != acp.AgentJaz {
				message := storage.NewInternalQueuedMessage(acp.CompletionPrompt(job, "the user"))
				// Work started for another bot reports to that bot while it is
				// still there.
				if output := session.Turn.ParentOutput; output != nil {
					if asker, err := s.Store.LoadSession(output.ReplyTo); err == nil && !asker.Archived {
						message.Text = bots.ResultPrompt(job)
						message.Output = output
					}
				}
				deliveries = append(deliveries, storage.TurnDelivery{SessionID: parent.ID, Message: message})
			}
		}
		if answer := bots.Answer(session); answer != nil {
			deliveries = append(deliveries, *answer)
		}
		output := session.Turn.Output
		// The run closes before its turn does; a restart in between finds the
		// run closed and only finishes the turn's bookkeeping.
		if run := loopRun(session); run != "" && err == nil {
			err = s.Loops.Finish(run, LoopRunStatus(job.State), job.Error, output != nil && (output.Shown || len(output.Replies) > 0))
		}
		if status == storage.StatusIdle && s.compactsWhenIdle(session) {
			deliveries = append(deliveries, storage.TurnDelivery{SessionID: job.ID, Message: storage.QueuedMessage{Action: storage.QueuedActionCompact}})
		}
		if err == nil {
			err = s.Store.FinishSessionTurn(job.ID, status, job.Error, time.Now().UTC(), deliveries)
		}
	} else if err == nil && job.State == acp.StateIdle {
		err = s.Store.CompleteSession(job.ID, time.Now().UTC())
	} else if err == nil {
		err = s.Store.UpdateSessionStatus(job.ID, status, job.Error, time.Time{})
	}
	unlock()
	if err != nil {
		return err
	}
	s.publishMessagesChanged(job.ID)
	s.publishSessionChanged(job.ID)
	for _, delivery := range deliveries {
		s.publishSessionChanged(delivery.SessionID)
		s.drainQueueSoon(delivery.SessionID)
	}
	if job.State == acp.StateIdle || job.State == acp.StateFailed {
		s.drainQueueSoon(job.ID)
	}
	return nil
}

// compactAt is how full, in percent, a bot's context may grow before its
// thread compacts when a turn leaves it idle. Providers compact on their own
// only at the edge of the window, in the middle of a turn.
const compactAt = 80

// compactsWhenIdle reports whether session, a bot's thread ending a turn with
// nothing queued behind it, is full enough to run the compact its agent
// offers. The end of a compact never queues another, so the thread compacts
// at most once per turn of growth, and a fill its provider has not reported
// never compacts it.
func (s *Server) compactsWhenIdle(session storage.Session) bool {
	usage := session.Usage
	return session.SourceType == storage.SourceBot && session.Turn.ActiveOperation != acp.ActiveOperationCompact &&
		len(session.QueuedMessages) == 0 && usage.ContextWindowTokens > 0 &&
		usage.ContextTokens*100 >= usage.ContextWindowTokens*compactAt && s.ACP.CanCompact(session.ID)
}

// loopRun is the routine run a thread's current turn does: the one its turn
// or claimed message names.
func loopRun(session storage.Session) string {
	if session.Turn == nil {
		return ""
	}
	if session.Turn.PendingMessage != nil {
		return session.Turn.PendingMessage.Output.LoopRun()
	}
	return session.Turn.Output.LoopRun()
}

// ResumeLoopRuns settles the routine runs the last process left open, once
// resumed turns and queues hold theirs again.
func (s *Server) ResumeLoopRuns(ctx context.Context) error {
	sessions, err := s.Store.ListSessions(storage.SessionFilter{Runtime: storage.RuntimeACP, IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		return err
	}
	held := map[string]bool{}
	for _, session := range sessions {
		for _, message := range session.QueuedMessages {
			held[message.Output.LoopRun()] = true
		}
		if session.Status == storage.StatusRunning {
			held[loopRun(session)] = true
		}
	}
	return s.Loops.Resume(ctx, held)
}

func (s *Server) ResumeQueuedTurns() error {
	sessions, err := s.Store.ListSessions(storage.SessionFilter{Runtime: storage.RuntimeACP, IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if takesQueuedTurns(session.Status) && len(session.QueuedMessages) > 0 {
			s.drainQueueSoon(session.ID)
		}
	}
	return nil
}

// FailInterruptedTurn ends an interrupted turn that cannot resume as failed,
// so the thread or bot waiting on it hears, as when a running turn fails.
func (s *Server) FailInterruptedTurn(_ context.Context, id, message string) error {
	session, err := s.Store.LoadSession(id)
	if err != nil {
		return err
	}
	job := acp.Job{ID: id, Slug: session.Slug, State: acp.StateFailed, Error: message}
	if session.RuntimeRef != nil {
		job.ACPAgent = session.RuntimeRef.Agent
	}
	return s.finishTurn(job)
}

func (s *Server) ResumeInterruptedTurn(ctx context.Context, id string) error {
	session, err := s.Store.LoadSession(id)
	if err != nil {
		return err
	}
	if run := loopRun(session); run != "" {
		closed, err := s.Loops.Closed(run)
		if err != nil {
			return err
		}
		if closed {
			// Recovery holds this thread's lock, which the bookkeeping takes.
			go s.HandleACPTurnFinished(context.WithoutCancel(ctx), acp.Job{ID: id, State: acp.StateIdle})
			return nil
		}
	}
	if session.Turn != nil && session.Turn.PendingMessage != nil {
		if session.Turn.PendingMessage.IsAction() {
			go s.runClaimedTurn(context.WithoutCancel(ctx), session, session.Turn.PendingMessage)
			return nil
		}
		return s.startQueuedPrompt(ctx, session, *session.Turn.PendingMessage)
	}
	return s.ACP.ResumeInterruptedTurn(ctx, id)
}
