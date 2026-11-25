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
	// activityShown is how many rows Activity lists.
	activityShown = 30
)

// Activity is what a bot did: a task it closed, or a thread it started, a
// side chat or a routine run in a thread of its own. Runs in the bot's own
// thread show in its chat, and messages between bots stay in its thread.
type Activity struct {
	// Kind is task, side_chat or routine.
	Kind string `json:"kind"`
	// At is when a running item started, or else when it happened or ended.
	At      time.Time `json:"at"`
	Running bool      `json:"running,omitempty"`
	// Title is the task's, side chat's or routine's name.
	Title string `json:"title"`
	// Text is a run's error.
	Text     string `json:"text,omitempty"`
	Status   string `json:"status,omitempty"`
	ThreadID string `json:"thread_id,omitempty"`
	LoopID   string `json:"loop_id,omitempty"`
	// Task is a task item's closed task.
	Task *Owed `json:"task,omitempty"`
}

// Owed is a commitment a bot took on: open, or closed when it is an
// Activity's Task.
type Owed struct {
	ID      int64  `json:"id"`
	Name    string `json:"name,omitempty"`
	Request string `json:"request"`
	// For names the bot it is owed to; empty for the user.
	For         string       `json:"for,omitempty"`
	Next        string       `json:"next,omitempty"`
	Status      string       `json:"status"`
	CheckAt     *time.Time   `json:"check_at,omitempty"`
	ReplySeq    int64        `json:"reply_seq,omitempty"`
	QuestionSeq int64        `json:"question_seq,omitempty"`
	Question    string       `json:"question,omitempty"`
	OpeningSeq  int64        `json:"opening_seq,omitempty"`
	DependsOn   []int64      `json:"depends_on,omitempty"`
	Blocked     bool         `json:"blocked,omitempty"`
	Moving      bool         `json:"moving,omitempty"`
	Stalled     bool         `json:"stalled,omitempty"`
	OpenedAt    time.Time    `json:"opened_at"`
	Threads     []OwedThread `json:"threads,omitempty"`
}

// OwedThread is a thread serving a commitment and how far it has got:
// working, background, finished, failed, idle or gone.
type OwedThread struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Title    string `json:"title"`
	State    string `json:"state"`
}

// ActivityView is what a bot owes, oldest first, and what it is doing and
// did. Started lists the side chats its side chats started, oldest first, by
// the one that started them.
type ActivityView struct {
	Items   []Activity            `json:"items"`
	Owed    []Owed                `json:"owed"`
	Started map[string][]Activity `json:"started"`
}

// Activity lists what a bot owes, then the newest activityShown things it is
// doing and did in the last activityWindow, newest first, a running thread
// dated by when it started. A thread serving a commitment shows through it,
// open or closed, unless it works again after the commitment closed; a side
// chat another side chat started shows under it. Only rows that show count.
func (s *Service) Activity(botID string) (ActivityView, error) {
	if !isBot(s.store, botID) {
		return ActivityView{}, storage.ErrBotNotFound
	}
	since := time.Now().Add(-activityWindow)
	open, err := s.store.ListOpenCommitments(botID)
	if err != nil {
		return ActivityView{}, err
	}
	view := ActivityView{Owed: make([]Owed, 0, len(open)), Started: map[string][]Activity{}}
	tasked := map[string]bool{}
	shownCommitments := make([]storage.Commitment, 0, len(open))
	for _, commitment := range open {
		owed, err := s.taskView(commitment)
		if err != nil {
			return ActivityView{}, err
		}
		for _, thread := range commitment.Threads {
			tasked[thread.ID] = true
		}
		view.Owed = append(view.Owed, owed)
		shownCommitments = append(shownCommitments, commitment)
	}
	closed, err := s.store.ListClosedCommitments(botID, since)
	if err != nil {
		return ActivityView{}, err
	}
	var out []Activity
	for i, commitment := range closed {
		for _, thread := range commitment.Threads {
			tasked[thread.ID] = tasked[thread.ID] || thread.State != "working" && thread.State != "background"
		}
		// Older closed tasks cannot make the cut, but still keep their threads
		// out of the list.
		if i >= activityShown {
			continue
		}
		task, err := s.taskView(commitment)
		if err != nil {
			return ActivityView{}, err
		}
		out = append(out, Activity{Kind: "task", At: commitment.ClosedAt, Title: cmp.Or(task.Name, task.Request), Status: commitment.Status, Task: &task})
		shownCommitments = append(shownCommitments, commitment)
	}
	var chats []storage.Session
	for _, archived := range []bool{false, true} {
		listed, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBotWorker, SourceID: botID, IncludeChildren: true, Archived: archived, UpdatedSince: since})
		if err != nil {
			return ActivityView{}, err
		}
		chats = append(chats, listed...)
	}
	out = append(out, collectSideChats(chats, shownCommitments, tasked, view.Started)...)
	routines, err := s.allRoutines()
	if err != nil {
		return ActivityView{}, err
	}
	for _, routine := range routines {
		if routine.BotID != botID {
			continue
		}
		runs, err := s.routines.ThreadRuns(routine.ID, activityShown)
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
	slices.SortStableFunc(out, func(a, b Activity) int { return b.At.Compare(a.At) })
	view.Items = out[:min(len(out), activityShown)]
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
	task := Owed{ID: commitment.ID, Name: commitment.Name, Request: commitment.Request, For: name, Next: commitment.Note,
		Status: commitment.Status, ReplySeq: commitment.ReplySeq, CheckAt: commitment.CheckAt, QuestionSeq: commitment.QuestionSeq, Question: commitment.Question, OpeningSeq: commitment.OpeningSeq, DependsOn: commitment.DependsOn,
		Blocked: commitment.Blocked, Moving: commitment.Moving, Stalled: commitment.Stalled, OpenedAt: commitment.OpenedAt}
	inTask := threadIDSet(commitment.Threads)
	for _, thread := range commitment.Threads {
		parentID := ""
		if thread.ParentID != thread.ID && inTask[thread.ParentID] {
			parentID = thread.ParentID
		}
		task.Threads = append(task.Threads, OwedThread{ID: thread.ID, ParentID: parentID, Title: cmp.Or(thread.Title, thread.Slug), State: thread.State})
	}
	return task, nil
}

// collectSideChats groups child worker chats under their parents in started
// (backfilling shown task descendants older than activityWindow) and returns
// the standalone side chats not already represented by a task.
func collectSideChats(chats []storage.Session, shownCommitments []storage.Commitment, tasked map[string]bool, started map[string][]Activity) []Activity {
	listed := make(map[string]bool, len(chats))
	for _, chat := range chats {
		listed[chat.ID] = true
	}
	for _, commitment := range shownCommitments {
		for _, thread := range commitment.Threads {
			listed[thread.ID] = true
		}
	}
	seen := map[string]map[string]bool{}
	record := func(parentID string, item Activity) {
		if seen[parentID] == nil {
			seen[parentID] = map[string]bool{}
		}
		if seen[parentID][item.ThreadID] {
			return
		}
		seen[parentID][item.ThreadID] = true
		started[parentID] = append(started[parentID], item)
	}
	var standalone []Activity
	for _, chat := range chats {
		item := Activity{Kind: "side_chat", At: cmp.Or(chat.CompletedAt, chat.UpdatedAt), Title: cmp.Or(chat.Title, chat.Slug), Status: chat.Status, ThreadID: chat.ID}
		if chat.Status == storage.StatusRunning && chat.Turn != nil {
			item.Running, item.At = true, cmp.Or(chat.Turn.StartedAt, chat.UpdatedAt)
		}
		if listed[chat.ParentID] {
			record(chat.ParentID, item)
			continue
		}
		if tasked[chat.ID] {
			continue
		}
		standalone = append(standalone, item)
	}
	for _, commitment := range shownCommitments {
		inTask := threadIDSet(commitment.Threads)
		for _, thread := range commitment.Threads {
			if thread.ParentID == "" || thread.ParentID == thread.ID || !inTask[thread.ParentID] {
				continue
			}
			record(thread.ParentID, Activity{
				Kind:     "side_chat",
				At:       cmp.Or(thread.CompletedAt, thread.UpdatedAt),
				Running:  thread.State == "working",
				Title:    cmp.Or(thread.Title, thread.Slug),
				Status:   thread.Status,
				ThreadID: thread.ID,
			})
		}
	}
	for _, children := range started {
		slices.SortStableFunc(children, func(a, b Activity) int { return a.At.Compare(b.At) })
	}
	return standalone
}

func threadIDSet(threads []storage.CommitmentThread) map[string]bool {
	ids := make(map[string]bool, len(threads))
	for _, thread := range threads {
		ids[thread.ID] = true
	}
	return ids
}
