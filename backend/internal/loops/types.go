package loops

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/promptmodule"
)

const (
	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusDeleted = "deleted"
)

const (
	RuntimeACP = "acp"
)

const (
	RunStatusStarting  = "starting"
	RunStatusRunning   = "running"
	RunStatusOK        = "ok"
	RunStatusError     = "error"
	RunStatusCancelled = "cancelled"
	RunStatusSkipped   = "skipped"
)

const (
	ScheduleCron  = "cron"
	ScheduleEvent = "event"
)

// Where a bot's routine runs: as a turn in the bot's own thread, or in a child
// thread of its own that reports back to the bot.
const (
	ThreadMain  = "main"
	ThreadChild = "child"
)

// What a routine does about slots and events it missed while Jaz was not
// running: skip them, or catch up with one run that covers them all.
const (
	MissedSkip    = "skip"
	MissedCatchUp = "catch_up"
)

const (
	TriggerGmail    = "gmail"
	TriggerWhatsApp = "whatsapp"
	TriggerTelegram = "telegram"
	TriggerSlack    = "slack"
	TriggerWebhook  = "webhook"
)

// Trigger fires a routine on an outside event instead of a schedule. The
// optional filters are case-insensitive substring matches.
type Trigger struct {
	Kind     string `json:"kind"`
	From     string `json:"from,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Contains string `json:"contains,omitempty"`
}

type Schedule struct {
	Kind     string `json:"kind"`
	Expr     string `json:"expr"`
	Timezone string `json:"timezone"`
}

type Loop struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Prompt   string   `json:"prompt"`
	Schedule Schedule `json:"schedule"`
	Status   string   `json:"status"`
	Runtime  string   `json:"runtime"`
	ACPAgent string   `json:"acp_agent,omitempty"`
	// ModelProvider/Model override the Settings > Agents defaults for runs.
	// Empty ReasoningEffort follows settings; "none" explicitly clears it.
	ModelProvider   string    `json:"model_provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Directory       string    `json:"directory,omitempty"`
	MemoryPath      string    `json:"memory_path,omitempty"`
	NextRunAt       time.Time `json:"next_run_at,omitempty"`
	LastRunAt       time.Time `json:"last_run_at,omitempty"`
	LastRunID       string    `json:"last_run_id,omitempty"`
	LastRunThreadID string    `json:"last_run_thread_id,omitempty"`
	LastRunStatus   string    `json:"last_run_status,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// BotID is the thread of the bot that owns this routine.
	BotID       string   `json:"bot_id"`
	Trigger     *Trigger `json:"trigger,omitempty"`
	WhenMissed  string   `json:"when_missed"`
	Thread      string   `json:"thread"`
	WebhookHash string   `json:"-"`
	// WebhookSecret is returned once, when a webhook trigger is created.
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type Run struct {
	ID           string    `json:"id"`
	LoopID       string    `json:"loop_id"`
	ThreadID     string    `json:"thread_id,omitempty"`
	ScheduledFor time.Time `json:"scheduled_for"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	// Reason says why the run happens when its schedule alone does not: the
	// events that fired it, or the slots it catches up on.
	Reason string `json:"reason,omitempty"`
}

// SeenEvent records that a run took an observed message, so the message fires
// its routine once.
type SeenEvent struct {
	LoopID string
	ID     string
	RunID  string
	At     time.Time
}

type CreateLoop struct {
	Name            string
	Prompt          string
	Schedule        Schedule
	Status          string
	Runtime         string
	ACPAgent        string
	ModelProvider   string
	Model           string
	ReasoningEffort string
	Directory       string
	BotID           string
	Trigger         *Trigger
	WhenMissed      string
	Thread          string
}

type UpdateLoop struct {
	Name            *string
	Prompt          *string
	Schedule        *Schedule
	Status          *string
	Runtime         *string
	ACPAgent        *string
	ModelProvider   *string
	Model           *string
	ReasoningEffort *string
	Directory       *string
	Trigger         *Trigger
	BotID           *string
	WhenMissed      *string
	Thread          *string
	Reschedule      bool
	RescheduleAt    time.Time
}

// BoardSummary identifies a Jaz board a loop's widget can be assigned to.
type BoardSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

// BoardService assigns a loop's widget to Jaz boards. Assignment is the widget
// enablement: a loop with no board has no widget. The widgets service
// implements it; loops only needs these loop-facing operations, which keeps
// widget types from leaking across the package boundary.
type BoardService interface {
	ListBoards() ([]BoardSummary, error)
	ValidateBoardIDs(boardIDs []string) error
	AssignLoopBoards(loop Loop, boardIDs []string) error
	BoardsForLoop(loopID string) ([]string, error)
}

type Repository interface {
	NewLoopID() string
	NewRunID() string
	LoadLoop(id string) (Loop, error)
	ListLoops() ([]Loop, error)
	ListRuns(loopID string, limit int) ([]Run, error)
	LoadRun(id string) (Run, error)
	ListDueLoopIDs(now time.Time) ([]string, error)
	ListActiveRunIDs() ([]string, error)
	// ActiveRuns lists a loop's pending and begun runs, oldest first.
	ActiveRuns(loopID string) ([]Run, error)
	LastFinishedAt(loopID, status string) (time.Time, error)
	EventSeen(loopID, eventID string) (bool, error)
	LatestEventAt(loopID string) (time.Time, error)
	SaveLoop(loop Loop) error
	SaveRun(run Run) error
	SaveLoops(loops []Loop) error
	// SaveRunAndLoop saves run, and with it loop when given and the messages
	// the run takes, in one write.
	SaveRunAndLoop(run Run, loop *Loop, taken ...SeenEvent) error
}

// RunController closes a run its executor could not start.
type RunController interface {
	Finish(runID, status, errText string) error
}

type Execution struct {
	Loop Loop
	Run  Run
	// Thread is ThreadMain for a turn in the loop's bot's thread, ThreadChild
	// for a child thread that reports to that bot, and empty for a thread of
	// the run's own.
	Thread                 string
	SystemPromptExtensions promptmodule.Modules
	ArtifactSurface        string
	Controller             RunController
}

type Executor interface {
	StartLoopRun(context.Context, Execution)
}
