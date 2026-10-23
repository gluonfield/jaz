package bots

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// CommitmentStore keeps what bots owe. A commitment closes as done only in
// the write that delivers it: DeliverMessage, a queued turn's or a finished
// turn's delivery.
type CommitmentStore interface {
	OpenCommitment(storage.Commitment) (storage.Commitment, error)
	UpdateCommitment(storage.Commitment) error
	DropCommitment(id int64, why string, at time.Time) error
	LoadCommitment(id int64) (storage.Commitment, error)
	ListOpenCommitments(botID string) ([]storage.Commitment, error)
	ListClosedCommitments(botID string, since time.Time) ([]storage.Commitment, error)
	DeliverMessage(event sessionevents.Event, closes []int64) (sessionevents.Event, error)
	AskCommitment(storage.Commitment, sessionevents.Event) (sessionevents.Event, error)
	ListWatchBots() ([]string, error)
}

type Commit struct {
	ID        int64
	Request   string
	Next      string
	Status    string
	Message   string
	DependsOn *[]int64
}

func (s *Service) Commit(threadID string, change Commit) (int64, error) {
	record, session, err := s.load(threadID)
	if err != nil || record.Kind != KindBot {
		return 0, errors.New("tasks are kept in a bot's own chat")
	}
	request := strings.TrimSpace(change.Request)
	next := strings.TrimSpace(change.Next)
	message := strings.TrimSpace(change.Message)
	if change.ID == 0 {
		if request == "" {
			return 0, errors.New("request is required to create a task")
		}
		requester := ""
		if session.Turn != nil && session.Turn.Output != nil {
			requester = session.Turn.Output.ReplyTo
		}
		commitment, err := s.store.OpenCommitment(storage.Commitment{BotID: threadID, Requester: requester, Request: request, Note: next})
		return commitment.ID, err
	}
	commitment, err := s.open(threadID, change.ID)
	if err != nil {
		return 0, err
	}
	status := cmp.Or(change.Status, commitment.Status)
	if status == storage.CommitmentDone || status == storage.CommitmentDropped {
		if message == "" || request != "" || next != "" || change.DependsOn != nil {
			return 0, errors.New("completing or cancelling requires status and message only")
		}
		if status == storage.CommitmentDone {
			return commitment.ID, s.Say(threadID, message, commitment.ID)
		}
		return commitment.ID, explain(s.store.DropCommitment(commitment.ID, message, time.Now()))
	}
	if status != storage.CommitmentPending && status != storage.CommitmentInProgress && status != storage.CommitmentWaitingForUser {
		return 0, fmt.Errorf("invalid task status %q", status)
	}
	if request != "" {
		commitment.Request = request
	}
	if next != "" {
		commitment.Note = next
	}
	if change.DependsOn != nil {
		commitment.DependsOn = *change.DependsOn
	}
	commitment.Status = status
	if status == storage.CommitmentWaitingForUser && message != "" {
		event, err := s.store.AskCommitment(commitment, sessionevents.Event{
			SessionID: threadID, Type: sessionevents.TypeRoomMessage, At: time.Now().UTC(),
			RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", BotID: threadID, Name: session.Title, Text: message},
		})
		if err == nil {
			s.events.Publish(event)
		}
		return commitment.ID, err
	}
	if message != "" {
		return 0, errors.New("message is used with waiting_for_user, completed or cancelled")
	}
	if status != storage.CommitmentWaitingForUser {
		commitment.QuestionSeq = 0
		commitment.QuestionUserSeq = 0
	}
	return commitment.ID, s.store.UpdateCommitment(commitment)
}

// delivers checks that a message from a bot's own thread reaching to, a
// thread or empty for the user, may close the commitments in done: each
// must be the bot's, open and owed to exactly that thread.
func (s *Service) delivers(threadID, to string, done []int64) error {
	for _, id := range done {
		thread, name, err := s.owed(threadID, id)
		if err != nil {
			return err
		}
		if thread != to {
			return fmt.Errorf("#%d is owed to %s; deliver it where they asked", id, cmp.Or(name, "the user"))
		}
	}
	return nil
}

// owed is the thread one of a bot's open commitments is owed to and who asked
// there.
func (s *Service) owed(botID string, id int64) (string, string, error) {
	commitment, err := s.open(botID, id)
	if err != nil {
		return "", "", err
	}
	return owedTo(s.store, commitment)
}

func (s *Service) open(botID string, id int64) (storage.Commitment, error) {
	commitment, err := s.store.LoadCommitment(id)
	if err == nil && (commitment.BotID != botID || !commitment.IsOpen()) {
		err = storage.ErrCommitmentNotOpen
	}
	if err != nil {
		return storage.Commitment{}, fmt.Errorf("#%d: %w", id, err)
	}
	return commitment, nil
}

// owedTo is the thread a commitment's result goes to and the name of who
// asked there; both empty when the user asked or that thread is gone.
func owedTo(store PromptStore, commitment storage.Commitment) (string, string, error) {
	thread, err := recipient(store, commitment.Requester)
	if thread == "" || err != nil {
		return "", "", err
	}
	_, name := author(store, thread)
	return thread, name, nil
}

// explain tells an agent what to do about a commitment that cannot close.
func explain(err error) error {
	if errors.Is(err, storage.ErrCommitmentWorking) {
		return fmt.Errorf("%w; wait for its result or stop it with stop_thread", err)
	}
	return err
}

// standing says what a commitment waits on, or that nothing moves on it.
func standing(commitment storage.Commitment) string {
	switch {
	case commitment.Status == storage.CommitmentWaitingForUser:
		return fmt.Sprintf(" · waiting on the user: question #%d", commitment.QuestionSeq)
	case commitment.Blocked:
		return fmt.Sprintf(" · prerequisites: %v", commitment.DependsOn)
	case commitment.Moving:
		return ""
	case commitment.Stalled:
		return " · stalled: no thread is working; resume this task"
	}
	return " · nothing moves on it"
}
