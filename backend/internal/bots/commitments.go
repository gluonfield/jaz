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
	MarkStalled(botID string) ([]storage.Commitment, error)
}

// Commit is a change a bot makes to its commitments: opening one when ID is
// zero, else a new request, next step or what it waits on, or dropping it.
// Waiting is requester, outside or none; empty leaves it as it is.
type Commit struct {
	ID      int64
	Request string
	Next    string
	Waiting string
	Drop    string
}

// waits maps what a bot says a commitment waits on to what is stored.
var waits = map[string]string{"requester": storage.WaitingRequester, "outside": storage.WaitingOutside, "none": ""}

// Commit applies change from a turn in a bot's own thread and returns the
// commitment's id. A commitment opens owed to whoever the turn answers.
func (s *Service) Commit(threadID string, change Commit) (int64, error) {
	record, session, err := s.load(threadID)
	if err != nil || record.Kind != KindBot {
		return 0, errors.New("commitments are kept in a bot's own chat")
	}
	var output storage.TurnOutput
	if session.Turn != nil && session.Turn.Output != nil {
		output = *session.Turn.Output
	}
	request, next, drop, wait := strings.TrimSpace(change.Request), strings.TrimSpace(change.Next), strings.TrimSpace(change.Drop), strings.TrimSpace(change.Waiting)
	waiting, ok := waits[wait]
	if !ok && wait != "" {
		return 0, fmt.Errorf("waiting is requester, outside or none, not %q", wait)
	}
	if change.ID == 0 {
		if request == "" {
			return 0, errors.New("request is required to open a commitment")
		}
		commitment, err := s.store.OpenCommitment(storage.Commitment{BotID: threadID, Requester: output.ReplyTo, Request: request, Note: next, Waiting: waiting})
		return commitment.ID, err
	}
	commitment, err := s.open(threadID, change.ID)
	if err != nil {
		return 0, err
	}
	if drop != "" {
		return commitment.ID, explain(s.store.DropCommitment(commitment.ID, drop, time.Now()))
	}
	if request != "" {
		commitment.Request = request
	}
	if next != "" {
		commitment.Note = next
	}
	if ok {
		commitment.Waiting = waiting
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
	if err == nil && (commitment.BotID != botID || commitment.Status != storage.CommitmentOpen) {
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

// NoteStore is what the note a bot's turn opens with is read from.
type NoteStore interface {
	PromptStore
	ListOpenCommitments(botID string) ([]storage.Commitment, error)
}

func SteerNote(store NoteStore) func(storage.Session) string {
	return func(thread storage.Session) string {
		if thread.SourceType != storage.SourceBot {
			return ""
		}
		return commitmentNote(store, thread.ID)
	}
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
		_, name, err := owedTo(store, commitment)
		if err != nil {
			return fmt.Sprintf("Your open commitments could not be read: %v.\n\n", err)
		}
		fmt.Fprintf(&b, "#%d for %s: %s", commitment.ID, cmp.Or(name, "the user"), commitment.Request)
		for _, thread := range commitment.Threads {
			fmt.Fprintf(&b, " · thread %s %s", thread.Slug, thread.State)
		}
		if commitment.Note != "" {
			fmt.Fprintf(&b, " · next: %s", commitment.Note)
		}
		b.WriteString(standing(commitment, cmp.Or(name, "the user")))
		b.WriteString("\n")
	}
	return b.String() + "\n"
}

// standing says what a commitment waits on, or that nothing moves on it.
func standing(commitment storage.Commitment, requester string) string {
	switch {
	case commitment.Waiting == storage.WaitingRequester:
		return " · waiting on " + requester
	case commitment.Waiting == storage.WaitingOutside:
		return " · waiting on an outside job"
	case commitment.Moving:
		return ""
	case commitment.Stalled:
		return " · stalled: nothing moved after Jaz reminded you, and the user was told"
	}
	return " · nothing moves on it"
}
