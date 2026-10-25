package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) CheckTaskAdmission(id string) error {
	return taskAdmission(context.Background(), botdb.New(s.db), id)
}

func taskAdmission(ctx context.Context, q *botdb.Queries, id string) error {
	stopped, err := q.StoppedTaskOwner(ctx, id)
	if err == nil && stopped {
		return storage.ErrTaskStopped
	}
	return err
}

func queuedTaskAdmission(ctx context.Context, q *botdb.Queries, thread string, message storage.QueuedMessage) error {
	if message.Kind == storage.QueuedMessageKindTaskWake {
		owed, err := q.HasOwedTasks(ctx, botdb.HasOwedTasksParams{BotID: thread, NowMs: time.Now().UnixMilli()})
		if err != nil {
			return err
		}
		if !owed {
			return storage.ErrNothingStalled
		}
	}
	if message.IsInternal() && message.From != "" {
		return taskAdmission(ctx, q, message.From)
	}
	return nil
}

func (s *Store) TransitionCommitment(task storage.Commitment, event sessionevents.Event) (sessionevents.Event, error) {
	if event.SessionID != task.BotID || event.Type != sessionevents.TypeRoomMessage || event.RoomMessage == nil || event.RoomMessage.Speaker != "bot" || strings.TrimSpace(event.RoomMessage.Text) == "" {
		return sessionevents.Event{}, errors.New("stopping or resuming requires an explanation in the bot's chat")
	}
	if task.Status != storage.CommitmentStopped && task.Status != storage.CommitmentPending {
		return sessionevents.Event{}, errors.New("a stopped task resumes only to pending")
	}
	task.Note = event.RoomMessage.Text
	events := []sessionevents.Event{event}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.appendSessionEventBatch(event.SessionID, nil, &task, events...)
	return events[0], err
}

func transitionCommitment(ctx context.Context, tx *sql.Tx, task storage.Commitment) error {
	q := botdb.New(tx)
	live, err := q.CommitmentLive(ctx, task.ID)
	if err != nil {
		return err
	}
	if live {
		return storage.ErrCommitmentWorking
	}
	staged, err := q.CommitmentStaged(ctx, task.ID)
	if err != nil {
		return err
	}
	if staged {
		return storage.ErrCommitmentStaged
	}
	updated, err := q.TransitionCommitment(ctx, botdb.TransitionCommitmentParams{ID: task.ID, BotID: task.BotID, Status: task.Status, Note: task.Note})
	if err != nil {
		return err
	}
	if updated == 0 {
		return storage.ErrCommitmentNotOpen
	}
	return retireCommitmentQueues(ctx, tx, task.ID, task.BotID, task.Status == storage.CommitmentPending)
}

func retireCommitmentQueues(ctx context.Context, tx *sql.Tx, taskID int64, botID string, includeWorkers bool) error {
	ids := []string{botID}
	if includeWorkers {
		workers, err := botdb.New(tx).CommitmentWorkerIDs(ctx, taskID)
		if err != nil {
			return err
		}
		ids = append(ids, workers...)
	}
	for _, id := range ids {
		if err := retireTaskQueue(ctx, tx, taskID, id, botID); err != nil {
			return err
		}
	}
	return nil
}

// Keep retired instructions as evidence without replaying them to the provider.
func retireTaskQueue(ctx context.Context, tx *sql.Tx, taskID int64, id, botID string) error {
	q := threaddb.New(tx)
	row, err := q.GetSession(ctx, id)
	if err != nil {
		return err
	}
	session, err := sessionFromDB(row)
	if err != nil {
		return err
	}
	var retired []storage.QueuedMessage
	workers, err := botdb.New(tx).CommitmentWorkerIDs(ctx, taskID)
	if err != nil {
		return err
	}
	bot := id == botID
	senders := make(map[string]bool, len(workers))
	for _, worker := range workers {
		senders[worker] = true
	}
	owned := func(message storage.QueuedMessage) bool {
		if bot {
			return message.IsInternal() && senders[message.From]
		}
		if message.From == "" {
			return message.IsInternal() && message.Output.LoopRun() == ""
		}
		return message.From == botID || senders[message.From]
	}
	kept := session.QueuedMessages[:0]
	for _, message := range session.QueuedMessages {
		retire := owned(message)
		if bot && message.Kind == storage.QueuedMessageKindTaskWake {
			owed, err := botdb.New(tx).HasOwedTasks(ctx, botdb.HasOwedTasksParams{BotID: id, NowMs: time.Now().UnixMilli()})
			if err != nil {
				return err
			}
			retire = !owed
		}
		if retire {
			retired = append(retired, message)
		} else {
			kept = append(kept, message)
		}
	}
	var pending *storage.QueuedMessage
	var turn *storage.Turn
	if !bot {
		if session.PendingSteer != nil && owned(*session.PendingSteer) {
			pending, session.PendingSteer = session.PendingSteer, nil
		}
		turn, session.Turn = session.Turn, nil
	}
	if len(retired) == 0 && pending == nil && turn == nil {
		return nil
	}
	if err := recordRetiredQueue(ctx, tx, id, retiredQueue{Task: taskID, Messages: retired, Pending: pending, Turn: turn}); err != nil {
		return err
	}
	session.QueuedMessages = kept
	return insertSession(tx, session)
}

type retiredQueue struct {
	Task     int64                   `json:"task,omitempty"`
	Messages []storage.QueuedMessage `json:"messages,omitempty"`
	Pending  *storage.QueuedMessage  `json:"pending,omitempty"`
	Turn     *storage.Turn           `json:"turn,omitempty"`
}

func recordRetiredQueue(ctx context.Context, tx *sql.Tx, id string, evidence retiredQueue) error {
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	eventq := eventdb.New(tx)
	seq, err := eventq.NextSessionEventSeq(ctx, id)
	if err != nil {
		return err
	}
	return insertSessionEvent(ctx, eventq, sessionevents.Event{SessionID: id, Seq: seq, Type: "task_queue_retired", Content: string(raw), At: time.Now().UTC()}, "")
}
