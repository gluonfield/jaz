package acp

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
)

// ErrClosed refuses new work while Jaz shuts down.
var ErrClosed = errors.New("jaz is restarting; try again once it is back")

// Start stores a new thread with first queued, claims it as the thread's turn
// and returns before the thread's agent starts. A restart before the agent
// has it starts it from the queue or the claim; while Jaz shuts down Start
// stores nothing.
func (m *Manager) Start(ctx context.Context, req SpawnRequest, first storage.QueuedMessage) (Job, error) {
	m.admission.RLock()
	defer m.admission.RUnlock()
	if m.closed {
		return Job{}, ErrClosed
	}
	var err error
	if first.ParentOutput, err = m.answering(first.From); err != nil {
		return Job{}, err
	}
	created, err := m.createSession(ctx, req, []storage.QueuedMessage{first})
	if err != nil {
		return Job{}, err
	}
	session := created.Session
	session.QueuedMessages = nil
	if err := m.store.ClaimQueuedTurn(session, first); err != nil {
		return Job{}, m.abandonSession(session, err)
	}
	go m.startClaimed(created, first)
	return jobFromSession(session, session.RuntimeRef.Agent, "", session.RuntimeRef.Cwd, StateStarting), nil
}

// startClaimed starts the agent, as the request that created the thread
// asked, and then the turn Start claimed; until that turn runs the thread is
// starting and refuses other sends. A turn that cannot start fails as a
// running one does, so whoever waits for it hears; one shutdown stops stays
// claimed for the next start.
func (m *Manager) startClaimed(created createdSession, first storage.QueuedMessage) {
	session := created.Session
	unlock := m.resumeLocks.Lock(session.ID)
	job, _, err := m.startSession(context.Background(), created, StateStarting)
	unlock()
	if err == nil {
		_, err = m.send(context.Background(), SendRequest{
			Session: session.ID, Message: first.Text, PlanRequested: first.PlanRequested, Completion: CompletionAsync,
			ParentVisible: first.SentByAgent(), FromAgent: first.SentByAgent(), ParentOutput: first.ParentOutput,
		}, sendOptions{transcript: sendTranscriptUserMessage, claimed: true})
	}
	if err == nil || m.isClosed() {
		return
	}
	m.log.Error("start thread", "session", session.ID, "error", err)
	if job != nil {
		job.setState(StateFailed, "", err.Error())
	}
	failed := jobFromSession(session, session.RuntimeRef.Agent, "", session.RuntimeRef.Cwd, StateFailed)
	failed.Error = err.Error()
	if m.TurnFinished != nil {
		m.TurnFinished(context.Background(), failed)
	}
	if first.SentByAgent() && !first.PlanRequested && m.Done != nil {
		go m.Done(context.Background(), failed)
	}
}

// answering is what sender's running turn answers, when it answers another
// bot; the result of work it starts goes on to answer it.
func (m *Manager) answering(sender string) (*storage.TurnOutput, error) {
	if sender == "" {
		return nil, nil
	}
	session, err := m.store.LoadSession(sender)
	if err != nil || session.Turn == nil {
		return nil, err
	}
	return session.Turn.Output.Continuation(), nil
}

func (m *Manager) isClosed() bool {
	m.admission.RLock()
	defer m.admission.RUnlock()
	return m.closed
}

// register makes a started agent's job live unless Jaz shuts down, so Close
// stops every process it did not refuse.
func (m *Manager) register(job *jobState, process *agentProcess) error {
	m.admission.RLock()
	defer m.admission.RUnlock()
	if m.closed {
		return ErrClosed
	}
	m.addJob(job, process)
	return nil
}
