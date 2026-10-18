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
// commitment.
type Activity struct {
	// Kind is side_chat, routine, commitment, or the kind of the bot activity
	// event.
	Kind string `json:"kind"`
	// At is when a running item started, or else when it happened or ended;
	// when an open commitment was opened.
	At      time.Time `json:"at"`
	Running bool      `json:"running,omitempty"`
	// Title is the side chat's or routine's name, the other bot's name, the
	// file's name or what a commitment was asked.
	Title string `json:"title"`
	// Text is a message's text, a run's error, or a commitment's next step or
	// why it was dropped.
	Text     string `json:"text,omitempty"`
	Status   string `json:"status,omitempty"`
	ThreadID string `json:"thread_id,omitempty"`
	LoopID   string `json:"loop_id,omitempty"`
	// For names the bot a commitment is owed to; empty for the user.
	For string `json:"for,omitempty"`
	// Threads serve a commitment.
	Threads []ActivityThread `json:"threads,omitempty"`
}

// ActivityThread is a thread serving a commitment and how far it has got.
type ActivityThread struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// State is working, finished, failed, idle or gone.
	State string `json:"state"`
}

// current reports whether an item stays at the top of Activity: what runs
// and what the bot still owes.
func (a Activity) current() bool {
	return a.Running || a.Kind == "commitment" && a.Status == storage.CommitmentOpen
}

// Activity lists what a bot is doing and owes, then what it did in the last
// activityWindow, newest first.
func (s *Service) Activity(botID string) ([]Activity, error) {
	if !s.isBot(botID) {
		return nil, storage.ErrBotNotFound
	}
	since := time.Now().Add(-activityWindow)
	var out []Activity
	for _, archived := range []bool{false, true} {
		chats, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBotWorker, SourceID: botID, IncludeChildren: true, Archived: archived, UpdatedSince: since})
		if err != nil {
			return nil, err
		}
		for _, chat := range chats {
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
	commitments, err := s.store.ListCommitments(botID, since)
	if err != nil {
		return nil, err
	}
	for _, commitment := range commitments {
		item := Activity{Kind: "commitment", At: cmp.Or(commitment.ClosedAt, commitment.OpenedAt), Title: commitment.Request, Text: commitment.Note, Status: commitment.Status}
		if to, name := owedTo(s.store, commitment); to != "" {
			item.For = name
		}
		for _, id := range commitment.Threads {
			thread, err := s.store.LoadSession(id)
			if err != nil {
				continue
			}
			state := threadState(thread)
			item.Threads = append(item.Threads, ActivityThread{ID: id, Title: cmp.Or(thread.Title, thread.Slug), State: state})
			item.Running = item.Running || state == "working" && commitment.Status == storage.CommitmentOpen
		}
		out = append(out, item)
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
	current := slices.DeleteFunc(slices.Clone(out), func(item Activity) bool { return !item.current() })
	done := slices.DeleteFunc(out, Activity.current)
	return append(current, done[:min(len(done), activityShown)]...), nil
}
