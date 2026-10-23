package bots

import (
	"cmp"
	"slices"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
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

// listedActivity says whether Activity lists a bot activity event: a message between
// bots or a change to MEMORY.md. Routine events are covered by the runs they
// announce.
func listedActivity(activity *sessionevents.BotActivityEvent) bool {
	switch activity.Kind {
	case "message_sent", "message_received":
		return true
	case "file_changed", "file_edited":
		return activity.Label == memoryFile
	}
	return false
}

// Activity is one thing a bot is doing or did: a side chat, a routine run,
// a message to or from another bot, a change to its MEMORY.md, or a
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
	For         string       `json:"for,omitempty"`
	Next        string       `json:"next,omitempty"`
	Status      string       `json:"status"`
	QuestionSeq int64        `json:"question_seq,omitempty"`
	DependsOn   []int64      `json:"depends_on,omitempty"`
	Blocked     bool         `json:"blocked,omitempty"`
	Stalled     bool         `json:"stalled,omitempty"`
	OpenedAt    time.Time    `json:"opened_at"`
	Threads     []OwedThread `json:"threads,omitempty"`
}

// OwedThread is a thread serving a commitment and how far it has got:
// working, background, finished, failed, idle or gone.
type OwedThread struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// ActivityView is what a bot owes, oldest first, and what it is doing and
// did. Started lists the side chats its side chats started, oldest first, by
// the one that started them.
type ActivityView struct {
	Items   []Activity            `json:"items"`
	Owed    []Owed                `json:"owed"`
	Started map[string][]Activity `json:"started"`
}

// Activity lists what a bot owes, then what it is doing and what it did in
// the last activityWindow, newest first. A thread serving a commitment shows
// through the commitment, unless it runs again after the commitment closed;
// a side chat another side chat started shows under it.
func (s *Service) Activity(botID string) (ActivityView, error) {
	if !isBot(s.store, botID) {
		return ActivityView{}, storage.ErrBotNotFound
	}
	since := time.Now().Add(-activityWindow)
	open, err := s.store.ListOpenCommitments(botID)
	if err != nil {
		return ActivityView{}, err
	}
	closed, err := s.store.ListClosedCommitments(botID, since)
	if err != nil {
		return ActivityView{}, err
	}
	view := ActivityView{Owed: make([]Owed, 0, len(open)), Started: map[string][]Activity{}}
	owing, served := map[string]bool{}, map[string]bool{}
	for _, commitment := range open {
		owed, err := s.taskView(commitment)
		if err != nil {
			return ActivityView{}, err
		}
		for _, thread := range commitment.Threads {
			owing[thread.ID] = true
		}
		view.Owed = append(view.Owed, owed)
	}
	var out []Activity
	for _, commitment := range closed {
		_, name, err := owedTo(s.store, commitment)
		if err != nil {
			return ActivityView{}, err
		}
		item := Activity{Kind: "commitment", At: commitment.ClosedAt, Title: commitment.Request, Status: commitment.Status, For: name}
		if commitment.Status == storage.CommitmentDropped {
			item.Text = commitment.Note
		}
		for _, thread := range commitment.Threads {
			item.ThreadID, served[thread.ID] = thread.ID, true
		}
		out = append(out, item)
	}
	var chats []storage.Session
	for _, archived := range []bool{false, true} {
		listed, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBotWorker, SourceID: botID, IncludeChildren: true, Archived: archived, UpdatedSince: since})
		if err != nil {
			return ActivityView{}, err
		}
		chats = append(chats, listed...)
	}
	listed := map[string]bool{}
	for _, chat := range chats {
		listed[chat.ID] = true
	}
	for _, chat := range chats {
		item := Activity{Kind: "side_chat", At: cmp.Or(chat.CompletedAt, chat.UpdatedAt), Title: chat.Title, Status: chat.Status, ThreadID: chat.ID}
		if chat.Status == storage.StatusRunning && chat.Turn != nil {
			item.Running, item.At = true, cmp.Or(chat.Turn.StartedAt, chat.UpdatedAt)
		}
		if listed[chat.ParentID] {
			view.Started[chat.ParentID] = append(view.Started[chat.ParentID], item)
			continue
		}
		if owing[chat.ID] || served[chat.ID] && !item.Running {
			continue
		}
		out = append(out, item)
	}
	for _, started := range view.Started {
		slices.SortStableFunc(started, func(a, b Activity) int { return a.At.Compare(b.At) })
	}
	routines, err := s.routines.List()
	if err != nil {
		return ActivityView{}, err
	}
	for _, routine := range routines {
		if routine.BotID != botID {
			continue
		}
		runs, err := s.routines.Runs(routine.ID, runsShown)
		if err != nil {
			return ActivityView{}, err
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
		return ActivityView{}, err
	}
	for _, event := range events {
		if activity := event.BotActivity; activity != nil && listedActivity(activity) {
			out = append(out, Activity{Kind: activity.Kind, At: event.At, Title: activity.Label, Text: activity.Text})
		}
	}
	slices.SortStableFunc(out, func(a, b Activity) int { return b.At.Compare(a.At) })
	running := slices.DeleteFunc(slices.Clone(out), func(item Activity) bool { return !item.Running })
	done := slices.DeleteFunc(out, func(item Activity) bool { return item.Running })
	view.Items = append(running, done[:min(len(done), activityShown)]...)
	return view, nil
}

func (s *Service) Tasks(botID string) ([]Owed, error) {
	record, _, err := s.load(botID)
	if err != nil || record.Kind != KindBot {
		return nil, storage.ErrBotNotFound
	}
	open, err := s.store.ListOpenCommitments(botID)
	if err != nil {
		return nil, err
	}
	tasks := make([]Owed, 0, len(open))
	for _, commitment := range open {
		task, err := s.taskView(commitment)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (s *Service) taskView(commitment storage.Commitment) (Owed, error) {
	_, name, err := owedTo(s.store, commitment)
	if err != nil {
		return Owed{}, err
	}
	task := Owed{ID: commitment.ID, Request: commitment.Request, For: name, Next: commitment.Note,
		Status: commitment.Status, QuestionSeq: commitment.QuestionSeq, DependsOn: commitment.DependsOn,
		Blocked: commitment.Blocked, Stalled: commitment.Stalled, OpenedAt: commitment.OpenedAt}
	for _, thread := range commitment.Threads {
		task.Threads = append(task.Threads, OwedThread{ID: thread.ID, Title: cmp.Or(thread.Title, thread.Slug), State: thread.State})
	}
	return task, nil
}
