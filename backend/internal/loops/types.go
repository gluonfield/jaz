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
	ScheduleCron = "cron"
)

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
	// ModelProvider/Model override the Settings > Agents defaults for runs;
	// empty follows settings at run time (like sessions).
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
	LoadRunByThread(threadID string) (Run, bool, error)
	ListDueLoopIDs(now time.Time) ([]string, error)
	ListActiveRunIDs() ([]string, error)
	HasActiveRun(loopID string) (bool, error)
	SaveLoop(loop Loop) error
	SaveRun(run Run) error
	SaveLoops(loops []Loop) error
	SaveLoopAndRun(loop Loop, run Run) error
	SaveRunAndLoop(run Run, loop *Loop) error
}

type RunController interface {
	MarkRunning(runID, threadID string) error
	Finish(runID, status, errText string) error
}

type Execution struct {
	Loop                   Loop
	Run                    Run
	Prompt                 string
	SystemPromptExtensions promptmodule.Modules
	ArtifactSurface        string
	Controller             RunController
}

type Executor interface {
	StartLoopRun(context.Context, Execution)
}
