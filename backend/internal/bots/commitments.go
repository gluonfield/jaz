package bots

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

// CommitmentStore keeps what bots owe.
type CommitmentStore interface {
	OpenCommitment(storage.Commitment) (storage.Commitment, error)
	UpdateCommitment(storage.Commitment) error
	CloseCommitment(id int64, status, note string, at time.Time) error
	LoadCommitment(id int64) (storage.Commitment, error)
	ListCommitments(botID string, closedSince time.Time) ([]storage.Commitment, error)
}

// Commit is a change a bot makes to its commitments: opening one when ID is
// zero, else a new request or next step, or dropping it.
type Commit struct {
	ID      int64
	Request string
	Next    string
	Drop    string
}

// Commit applies change from a turn in a bot's own thread and returns the
// commitment's id. A commitment opens owed to whoever the turn answers.
func (s *Service) Commit(threadID string, change Commit) (int64, error) {
	session, err := s.store.LoadSession(threadID)
	if err != nil {
		return 0, err
	}
	if session.SourceType != storage.SourceBot || !s.isBot(threadID) {
		return 0, errors.New("commitments are kept in a bot's own chat")
	}
	request, next, drop := strings.TrimSpace(change.Request), strings.TrimSpace(change.Next), strings.TrimSpace(change.Drop)
	if change.ID == 0 {
		if request == "" {
			return 0, errors.New("request is required to open a commitment")
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
	if drop != "" {
		if err := s.idle(commitment); err != nil {
			return 0, err
		}
		return commitment.ID, s.store.CloseCommitment(commitment.ID, storage.CommitmentDropped, drop, time.Now())
	}
	if request != "" {
		commitment.Request = request
	}
	if next != "" {
		commitment.Note = next
	}
	return commitment.ID, s.store.UpdateCommitment(commitment)
}

// delivers checks that a message from a bot's own thread to destination,
// the bot or thread it reaches or empty for the user, may close done.
func (s *Service) delivers(threadID, destination string, done []int64) error {
	for _, id := range done {
		commitment, err := s.open(threadID, id)
		if err != nil {
			return err
		}
		if to, name := owedTo(s.store, commitment); to != destination {
			return fmt.Errorf("#%d is owed to %s; deliver it there", id, name)
		}
		if err := s.idle(commitment); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) open(botID string, id int64) (storage.Commitment, error) {
	commitment, err := s.store.LoadCommitment(id)
	if err == nil && (commitment.BotID != botID || commitment.Status != storage.CommitmentOpen) {
		err = storage.ErrCommitmentNotOpen
	}
	if err != nil {
		return storage.Commitment{}, fmt.Errorf("#%d: %w", id, err)
	}
	return commitment, nil
}

// idle refuses closing a commitment while one of its threads still works.
func (s *Service) idle(commitment storage.Commitment) error {
	for _, id := range commitment.Threads {
		if thread, err := s.store.LoadSession(id); err == nil && threadState(thread) == "working" {
			return fmt.Errorf("#%d: thread %s is still working; wait for its result or stop it with stop_thread", commitment.ID, thread.Slug)
		}
	}
	return nil
}

// owedTo is who a commitment's result goes to: the bot or thread behind its
// requester while it is there, else the user, as an empty id.
func owedTo(store PromptStore, commitment storage.Commitment) (string, string) {
	return destination(store, commitment.Requester)
}

// threadState is how far a thread has got with its work.
func threadState(thread storage.Session) string {
	switch {
	case thread.Archived:
		return "gone"
	case thread.Status == storage.StatusRunning || thread.Status == storage.StatusInterrupted || len(thread.QueuedMessages) > 0:
		return "working"
	case thread.Status == storage.StatusError:
		return "failed"
	case !thread.CompletedAt.IsZero():
		return "finished"
	}
	return "idle"
}

// NoteStore is what the note a bot's turn opens with is read from.
type NoteStore interface {
	PromptStore
	ListCommitments(botID string, closedSince time.Time) ([]storage.Commitment, error)
}

// commitmentNote lists a bot's open commitments for the turn its own thread
// opens with; empty when it has none.
func commitmentNote(store NoteStore, bot string) string {
	commitments, err := store.ListCommitments(bot, time.Now())
	open := slices.DeleteFunc(commitments, func(commitment storage.Commitment) bool { return commitment.Status != storage.CommitmentOpen })
	if err != nil || len(open) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Your open commitments. Deliver each result with done set to its number; drop what is no longer wanted:\n")
	for _, commitment := range open {
		_, to := owedTo(store, commitment)
		fmt.Fprintf(&b, "#%d for %s: %s", commitment.ID, to, commitment.Request)
		for _, id := range commitment.Threads {
			if thread, err := store.LoadSession(id); err == nil {
				fmt.Fprintf(&b, " · thread %s %s", thread.Slug, threadState(thread))
			}
		}
		if commitment.Note != "" {
			fmt.Fprintf(&b, " · next: %s", commitment.Note)
		}
		b.WriteString("\n")
	}
	return b.String() + "\n"
}
