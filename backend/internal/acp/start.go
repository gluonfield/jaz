package acp

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
)

// Start stores a new thread with first queued, claims it as the thread's turn
// and returns before the thread's agent starts. A restart before the agent
// has it starts it from the queue or the claim; while Jaz shuts down Start
// stores nothing.
func (m *Manager) Start(ctx context.Context, req SpawnRequest, first storage.QueuedMessage) (Job, error) {
	release, err := m.admit()
	if err != nil {
		return Job{}, err
	}
	defer release()
	if first.ParentOutput, err = m.answering(first.From); err != nil {
		return Job{}, err
	}
	queue := storage.CanonicalQueuedMessages([]storage.QueuedMessage{first})
	if len(queue) == 0 {
		return Job{}, errors.New("initial message is required")
	}
	first = queue[0]
	created, err := m.createSession(ctx, req, queue)
	if err != nil {
		return Job{}, err
	}
	created.Session.QueuedMessages = nil
	session := created.Session
	if err := m.store.ClaimQueuedTurn(session, first); err != nil {
		return Job{}, m.abandonSession(session, err)
	}
	if m.ThreadStarted != nil {
		m.ThreadStarted(session, first.Text)
	}
	job := jobFromSession(session, session.RuntimeRef.Agent, "", session.RuntimeRef.Cwd, StateStarting)
	go m.startClaimed(created, first)
	return job, nil
}

// startClaimed starts the agent, as the request that created the thread
// asked, and then the turn Start claimed; until that turn runs the thread is
// starting and refuses other sends. A turn that cannot start fails as a
// running one does, so whoever waits for it hears; one shutdown stops stays
// claimed for the next start.
func (m *Manager) startClaimed(created createdSession, first storage.QueuedMessage) {
	ctx, finish, err := m.beginLaunch(context.Background())
	if err != nil {
		return
	}
	defer finish()
	session := created.Session
	unlock := m.resumeLocks.Lock(session.ID)
	job, err := m.startSession(ctx, created, StateStarting)
	unlock()
	if err == nil {
		_, err = m.send(ctx, SendRequest{
			Session: session.ID, Message: first.Text, PlanRequested: first.PlanRequested, Completion: CompletionAsync,
			ParentVisible: first.SentByAgent(), FromUser: !first.SentAt.IsZero(), ParentOutput: first.ParentOutput,
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
