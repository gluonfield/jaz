package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/storage"
)

type acpTurnKind int

const (
	acpTurnPrompt acpTurnKind = iota
	acpTurnCompact
)

type acpStreamTurn struct {
	Kind          acpTurnKind
	Message       string
	Contexts      []storage.MessageContext
	Attachments   []storage.Attachment
	PlanRequested bool
	GoalRequested bool
}

type acpStreamStatusReader interface {
	StreamStatus(string) (acp.Job, error)
}

func acpStreamTurnFromRequest(req streamRequest) acpStreamTurn {
	turn := acpStreamTurn{
		Kind:          acpTurnPrompt,
		Message:       req.Message,
		PlanRequested: req.PlanRequested,
		GoalRequested: req.GoalRequested,
	}
	if isACPCompactCommand(req.Message) {
		turn.Kind = acpTurnCompact
		turn.GoalRequested = false
	}
	return turn
}

func (t acpStreamTurn) compact() bool {
	return t.Kind == acpTurnCompact
}

func (s *Server) streamACPSession(w http.ResponseWriter, flusher http.Flusher, clientCtx context.Context, session storage.Session, turn acpStreamTurn) {
	if s.ACP == nil {
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: "acp manager is not configured"})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}

	var err error
	turnTitle := turn.Message
	if turn.compact() {
		turnTitle = "Compact"
	}
	session, err = s.beginACPTurn(clientCtx, session, turnTitle)
	if err != nil {
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}
	startCtx, cancelStart := serverActionContextFrom(clientCtx)
	var job acp.Job
	if turn.compact() {
		job, err = s.ACP.Compact(startCtx, acp.CompactRequest{Session: session.ID})
	} else {
		job, err = s.ACP.Send(startCtx, acp.SendRequest{
			Session:       session.ID,
			Message:       turn.Message,
			Contexts:      turn.Contexts,
			Attachments:   turn.Attachments,
			Completion:    acp.CompletionInline,
			PlanRequested: turn.PlanRequested,
			GoalRequested: turn.GoalRequested,
		})
	}
	cancelStart()
	if err != nil {
		sendErr := acpSendError(session, err)
		s.logger().Error("acp send failed", "session", session.ID, "error", sendErr)
		s.setSessionError(session, sendErr.Error())
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: sendErr.Error()})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}

	emittedAssistant := 0
	emittedThought := 0
	seenTools := map[string]struct{}{}
	status := s.ACP.Status
	if reader, ok := s.ACP.(acpStreamStatusReader); ok {
		status = reader.StreamStatus
	}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for {
		emitACPJob(w, flusher, job, &emittedAssistant, &emittedThought, seenTools)
		if job.State == acp.StateFailed {
			s.setSessionError(session, job.Error)
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: job.Error})
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
			return
		}
		if isACPTerminal(job.State) {
			s.setSessionStatus(session, storage.StatusIdle)
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
			return
		}
		select {
		case <-clientCtx.Done():
			return
		case <-ticker.C:
			job, err = status(session.ID)
			if err != nil {
				s.setSessionError(session, err.Error())
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
				return
			}
		}
	}
}

func (s *Server) beginACPTurn(ctx context.Context, session storage.Session, message string) (storage.Session, error) {
	unlock := s.lockSession(session.ID)
	defer unlock()

	current, err := s.Store.LoadSession(session.ID)
	if err != nil {
		return session, err
	}
	session = current
	if s.sessionRuntimeRunning(session) {
		return session, fmt.Errorf("session %s is already running", session.Slug)
	}
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		return session, err
	}
	markSessionRunning(&session)
	if session.Title == "" {
		session.Title = titleFromMessage(message)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return session, err
	}
	s.maybeGenerateSessionTitle(session, message)
	s.publishSessionChanged(session.ID)
	return session, nil
}

func emitACPJob(w http.ResponseWriter, flusher http.Flusher, job acp.Job, emittedAssistant, emittedThought *int, seenTools map[string]struct{}) {
	for _, call := range job.ToolCalls {
		key := firstNonEmpty(call.ID, call.Title)
		if key == "" {
			continue
		}
		if _, ok := seenTools[key]; ok {
			continue
		}
		seenTools[key] = struct{}{}
		writeSSE(w, flusher, agent.StreamEvent{
			Type:     agent.StreamToolCall,
			ToolName: firstNonEmpty(call.Title, call.ID),
		})
	}
	if *emittedAssistant < len(job.Assistant) {
		delta := job.Assistant[*emittedAssistant:]
		*emittedAssistant = len(job.Assistant)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDelta, Delta: delta})
	}
	if *emittedThought < len(job.Thought) {
		delta := job.Thought[*emittedThought:]
		*emittedThought = len(job.Thought)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamReasoning, Reasoning: delta})
	}
}

func isACPTerminal(state string) bool {
	return state == acp.StateIdle || state == acp.StateCancelled
}

func acpSendError(session storage.Session, err error) error {
	if strings.Contains(err.Error(), "active acp session not found") {
		return fmt.Errorf("acp session %q (%s) could not be resumed: %v", session.Slug, session.ID, err)
	}
	return err
}
