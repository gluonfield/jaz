package bots

import (
	"errors"
	"fmt"
	"slices"
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
	if !isBot(s.store, threadID) {
		return 0, errors.New("commitments are kept in a bot's own chat")
	}
	session, err := s.store.LoadSession(threadID)
	if err != nil {
		return 0, err
	}
	var output storage.TurnOutput
	if session.Turn != nil && session.Turn.Output != nil {
		output = *session.Turn.Output
	}
	request, next, drop := strings.TrimSpace(change.Request), strings.TrimSpace(change.Next), strings.TrimSpace(change.Drop)
	if change.ID == 0 {
		if request == "" {
			return 0, errors.New("request is required to open a commitment")
		}
		commitment, err := s.store.OpenCommitment(storage.Commitment{BotID: threadID, Requester: output.ReplyTo, Request: request, Note: next})
		return commitment.ID, err
	}
	commitment, err := s.open(threadID, change.ID)
	if err != nil {
		return 0, err
	}
	if drop != "" {
		if slices.Contains(output.Done, commitment.ID) {
			return 0, fmt.Errorf("#%d is delivered by this turn's answer", commitment.ID)
		}
		if err := s.store.DropCommitment(commitment.ID, drop, time.Now()); err != nil {
			return 0, fmt.Errorf("#%d: %w", commitment.ID, err)
		}
		return commitment.ID, nil
	}
	if request != "" {
		commitment.Request = request
	}
	if next != "" {
		commitment.Note = next
	}
	return commitment.ID, s.store.UpdateCommitment(commitment)
}

// delivers checks that a message from a bot's own thread reaching to, a
// thread or empty for the user, may close the commitments in done: each
// must be the bot's, open and owed to exactly that thread.
func (s *Service) delivers(threadID, to string, done []int64) error {
	for _, id := range done {
		commitment, err := s.open(threadID, id)
		if err != nil {
			return err
		}
		if recipient(s.store, commitment.Requester) != to {
			return fmt.Errorf("#%d is owed to %s; deliver it where they asked", id, owedName(s.store, commitment))
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

// owedName names who a commitment is owed to: the bot that asked, or the user
// when it was the user or the bot is gone.
func owedName(store PromptStore, commitment storage.Commitment) string {
	if recipient(store, commitment.Requester) == "" {
		return "the user"
	}
	_, name := author(store, commitment.Requester)
	return name
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
	ListOpenCommitments(botID string) ([]storage.Commitment, error)
}

// commitmentNote lists a bot's open commitments for the turn its own thread
// opens with; empty when it has none.
func commitmentNote(store NoteStore, bot string) string {
	open, err := store.ListOpenCommitments(bot)
	if err != nil {
		return fmt.Sprintf("Your open commitments could not be read: %v.\n\n", err)
	}
	if len(open) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Your open commitments. Deliver each result with done set to its number; drop what is no longer wanted:\n")
	for _, commitment := range open {
		fmt.Fprintf(&b, "#%d for %s: %s", commitment.ID, owedName(store, commitment), commitment.Request)
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
