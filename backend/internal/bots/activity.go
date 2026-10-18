package bots

import (
	"cmp"
	"slices"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	// activityWindow is how far back Activity looks.
	activityWindow = 30 * 24 * time.Hour
	// runsShown is how many recent runs of each routine Activity lists.
	runsShown = 20
	// activityShown bounds what Activity lists besides what is running.
	activityShown = 200
)

// listedActivity are the bot activity events Activity lists; routine events
// are covered by the runs they announce.
var listedActivity = []string{"message_sent", "message_received", "file_changed", "file_edited"}

// Activity is one thing a bot is doing or did: a side chat, a routine run,
// a message to or from another bot, a change to its identity files, or a
// commitment it closed.
type Activity struct {
	// Kind is side_chat, routine, commitment, or the kind of the bot activity
	// event.
	Kind string `json:"kind"`
	// At is when a running item started, or else when it happened or ended.
	At      time.Time `json:"at"`
	Running bool      `json:"running,omitempty"`
	// Title is the side chat's or routine's name, the other bot's name, the
	// file's name or what a commitment asked.
	Title string `json:"title"`
	// Text is a message's text, a run's error or why a commitment was dropped.
	Text     string `json:"text,omitempty"`
	Status   string `json:"status,omitempty"`
	ThreadID string `json:"thread_id,omitempty"`
	LoopID   string `json:"loop_id,omitempty"`
	// For names the bot a commitment was owed to; empty for the user.
	For string `json:"for,omitempty"`
}

// Owed is a commitment a bot has not closed yet.
type Owed struct {
	ID      int64  `json:"id"`
	Request string `json:"request"`
	// For names the bot it is owed to; empty for the user.
	For      string       `json:"for,omitempty"`
	Next     string       `json:"next,omitempty"`
	OpenedAt time.Time    `json:"opened_at"`
	Threads  []OwedThread `json:"threads,omitempty"`
}

// OwedThread is a thread serving a commitment and how far it has got:
// working, finished, failed, idle or gone.
type OwedThread struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// Owed lists what a bot still owes, oldest first.
func (s *Service) Owed(botID string) ([]Owed, error) {
	if !isBot(s.store, botID) {
		return nil, storage.ErrBotNotFound
	}
	open, err := s.store.ListOpenCommitments(botID)
	if err != nil {
		return nil, err
	}
	out := make([]Owed, 0, len(open))
	for _, commitment := range open {
		owed := Owed{ID: commitment.ID, Request: commitment.Request, For: forName(s.store, commitment), Next: commitment.Note, OpenedAt: commitment.OpenedAt}
		for _, id := range commitment.Threads {
			if thread, err := s.store.LoadSession(id); err == nil {
				owed.Threads = append(owed.Threads, OwedThread{ID: id, Title: cmp.Or(thread.Title, thread.Slug), State: threadState(thread)})
			}
		}
		out = append(out, owed)
	}
	return out, nil
}

// forName is who a commitment is owed to as Activity names them: the bot, or
// empty for the user.
func forName(store PromptStore, commitment storage.Commitment) string {
	if recipient(store, commitment.Requester) == "" {
		return ""
	}
	return owedName(store, commitment)
}

// Activity lists what a bot is doing, then what it did in the last
// activityWindow, newest first. A thread serving a commitment shows through
// the commitment instead.
func (s *Service) Activity(botID string) ([]Activity, error) {
	if !isBot(s.store, botID) {
		return nil, storage.ErrBotNotFound
	}
	since := time.Now().Add(-activityWindow)
	open, err := s.store.ListOpenCommitments(botID)
	if err != nil {
		return nil, err
	}
	closed, err := s.store.ListClosedCommitments(botID, since)
	if err != nil {
		return nil, err
	}
	var out []Activity
	served := map[string]bool{}
	for _, commitment := range append(open, closed...) {
		for _, thread := range commitment.Threads {
			served[thread] = true
		}
	}
	for _, commitment := range closed {
		item := Activity{Kind: "commitment", At: commitment.ClosedAt, Title: commitment.Request, Status: commitment.Status, For: forName(s.store, commitment)}
		if commitment.Status == storage.CommitmentDropped {
			item.Text = commitment.Note
		}
		if n := len(commitment.Threads); n > 0 {
			item.ThreadID = commitment.Threads[n-1]
		}
		out = append(out, item)
	}
	for _, archived := range []bool{false, true} {
		chats, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBotWorker, SourceID: botID, IncludeChildren: true, Archived: archived, UpdatedSince: since})
		if err != nil {
			return nil, err
		}
		for _, chat := range chats {
			if served[chat.ID] {
				continue
			}
			item := Activity{Kind: "side_chat", At: cmp.Or(chat.CompletedAt, chat.UpdatedAt), Title: chat.Title, Status: chat.Status, ThreadID: chat.ID}
			if chat.Status == storage.StatusRunning && chat.Turn != nil {
				item.Running, item.At = true, cmp.Or(chat.Turn.StartedAt, chat.UpdatedAt)
			}
			out = append(out, item)
		}
	}
	routines, err := s.routines.List()
	if err != nil {
		return nil, err
	}
	for _, routine := range routines {
		if routine.BotID != botID {
			continue
		}
		runs, err := s.routines.Runs(routine.ID, runsShown)
		if err != nil {
			return nil, err
		}
		for _, run := range runs {
			item := Activity{
				Kind:     "routine",
				At:       cmp.Or(run.FinishedAt, run.StartedAt, run.ScheduledFor),
				Running:  run.Status == loops.RunStatusStarting || run.Status == loops.RunStatusRunning,
				Title:    routine.Name,
				Text:     run.Error,
				Status:   run.Status,
				ThreadID: run.ThreadID,
				LoopID:   routine.ID,
			}
			if item.Running || item.At.After(since) {
				out = append(out, item)
			}
		}
	}
	events, err := s.store.LoadBotActivity(botID, since)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if activity := event.BotActivity; activity != nil && slices.Contains(listedActivity, activity.Kind) {
			out = append(out, Activity{Kind: activity.Kind, At: event.At, Title: activity.Label, Text: activity.Text})
		}
	}
	slices.SortStableFunc(out, func(a, b Activity) int { return b.At.Compare(a.At) })
	running := slices.DeleteFunc(slices.Clone(out), func(item Activity) bool { return !item.Running })
	done := slices.DeleteFunc(out, func(item Activity) bool { return item.Running })
	return append(running, done[:min(len(done), activityShown)]...), nil
}
