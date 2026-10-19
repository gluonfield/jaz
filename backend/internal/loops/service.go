package loops

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/templates/looprun"
)

type Service struct {
	Repo        Repository
	Executor    Executor
	Log         *log.Logger
	Now         func() time.Time
	MemoryPaths *MemoryPaths
	// PromptExtras append sections, such as widget instructions or the
	// owning bot's saved prompt, to the metadata prompt of each run.
	PromptExtras    []func(Loop, Run) string
	ArtifactSurface func(Loop, Run) string
	// RunCheck can fail a run whose turn ended well but left its work undone,
	// as a widget run that published nothing.
	RunCheck func(Loop, Run) error
	// Notice posts a bot's message in its own chat.
	Notice func(botID, text string) error
	// ModelCheck refuses a routine whose runs would not start on the model it
	// names.
	ModelCheck func(Loop) error
	mu         sync.Mutex
	// ours holds the runs this process created and has not finished, which a
	// previous process cannot have left behind.
	ours map[string]bool
	// away holds bots' routines while the user is away; see GuardAway.
	away bool
	// unsettled says why scheduled runs wait: the runs a previous process left
	// open are not settled after a failed try. See settle.
	unsettled string
}

type RuntimePromptBuilder struct {
	Repo         Repository
	PromptExtras []func(Loop, Run) string
}

func (b RuntimePromptBuilder) For(loop Loop, run Run, now time.Time) (promptmodule.Modules, error) {
	var extras []string
	for _, section := range b.PromptExtras {
		if extra := strings.TrimSpace(section(loop, run)); extra != "" {
			extras = append(extras, extra)
		}
	}
	previous, err := b.Repo.RunBefore(run.ID)
	if err != nil {
		return nil, err
	}
	return SystemPromptExtensions(loop, run, previous, now, extras...), nil
}

func (b RuntimePromptBuilder) ForRun(runID string, now time.Time) (promptmodule.Modules, error) {
	run, err := b.Repo.LoadRun(runID)
	if err != nil {
		return nil, err
	}
	loop, err := b.Repo.LoadLoop(run.LoopID)
	if err != nil {
		return nil, err
	}
	return b.For(loop, run, now)
}

type Option func(*Service)

func WithMemoryPaths(paths *MemoryPaths) Option {
	return func(s *Service) {
		s.MemoryPaths = paths
	}
}

func WithPromptExtras(extras ...func(Loop, Run) string) Option {
	return func(s *Service) {
		s.PromptExtras = append(s.PromptExtras, extras...)
	}
}

func WithArtifactSurface(surface func(Loop, Run) string) Option {
	return func(s *Service) {
		s.ArtifactSurface = surface
	}
}

func WithRunCheck(check func(Loop, Run) error) Option {
	return func(s *Service) {
		s.RunCheck = check
	}
}

func WithModelCheck(check func(Loop) error) Option {
	return func(s *Service) {
		s.ModelCheck = check
	}
}

func NewService(repo Repository, executor Executor, logger *log.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = log.Default()
	}
	service := &Service{Repo: repo, Executor: executor, Log: logger.WithPrefix("loops"), ours: map[string]bool{}}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func (s *Service) RuntimePromptBuilder() RuntimePromptBuilder {
	return RuntimePromptBuilder{Repo: s.Repo, PromptExtras: s.PromptExtras}
}

func (s *Service) Create(input CreateLoop) (Loop, error) {
	now := s.now()
	input, nextRun, err := NormalizeCreate(input, now)
	if err != nil {
		return Loop{}, err
	}
	loop := Loop{
		ID:              s.Repo.NewLoopID(),
		Name:            input.Name,
		Prompt:          input.Prompt,
		Schedule:        input.Schedule,
		Status:          input.Status,
		Runtime:         input.Runtime,
		ACPAgent:        input.ACPAgent,
		ModelProvider:   input.ModelProvider,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		Directory:       input.Directory,
		NextRunAt:       nextRun,
		CreatedAt:       now,
		UpdatedAt:       now,
		BotID:           input.BotID,
		Trigger:         input.Trigger,
		WhenMissed:      input.WhenMissed,
		Thread:          input.Thread,
		KeepRunning:     input.KeepRunning,
	}
	if err := s.checkModel(Loop{}, loop); err != nil {
		return Loop{}, err
	}
	ensureWebhookSecret(&loop)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.assignMemoryPathLocked(&loop); err != nil {
		return Loop{}, err
	}
	return loop, s.Repo.SaveLoop(loop)
}

func (s *Service) Load(id string) (Loop, error) {
	loop, err := s.Repo.LoadLoop(id)
	if err != nil {
		return Loop{}, err
	}
	return s.blocked([]Loop{loop})[0], nil
}

func (s *Service) List() ([]Loop, error) {
	items, err := s.Repo.ListLoops()
	return s.blocked(items), err
}

// blocked tells each active scheduled loop why its runs wait, if they do.
func (s *Service) blocked(items []Loop) []Loop {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range items {
		if items[i].Status == StatusActive && items[i].Schedule.Kind == ScheduleCron {
			items[i].Blocked = s.unsettled
			if items[i].Blocked == "" && s.heldAway(items[i]) {
				items[i].Blocked = "Waiting until you write to Jaz again. You have been away for 3 days."
			}
		}
	}
	return items
}

func (s *Service) Update(id string, input UpdateLoop) (Loop, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.Repo.LoadLoop(id)
	if err != nil {
		return Loop{}, err
	}
	if current.Status == StatusDeleted {
		return Loop{}, fmt.Errorf("loop %s has been deleted", id)
	}
	if input.Status != nil && current.Status != StatusActive && *input.Status == StatusActive {
		input.Reschedule = true
	}
	next, _, err := NormalizeUpdate(current, input, now)
	if err != nil {
		return Loop{}, err
	}
	if err := s.checkModel(current, next); err != nil {
		return Loop{}, err
	}
	next.UpdatedAt = now
	ensureWebhookSecret(&next)
	if err := s.Repo.SaveLoop(next); err != nil {
		return Loop{}, err
	}
	saved, err := s.Repo.LoadLoop(id)
	saved.WebhookSecret = next.WebhookSecret
	return saved, err
}

// checkModel refuses the model next's runs would start on when the request
// chose it. A model the routine saved earlier stays as it is: a plan change can
// withdraw one, and its runs then start on the agent's default.
func (s *Service) checkModel(current, next Loop) error {
	unchanged := next.ACPAgent == current.ACPAgent && next.ModelProvider == current.ModelProvider && next.Model == current.Model
	if s.ModelCheck == nil || next.Model == "" || unchanged {
		return nil
	}
	return s.ModelCheck(next)
}

func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	loop, err := s.Repo.LoadLoop(id)
	if err != nil {
		return err
	}
	if loop.Status == StatusDeleted {
		return nil
	}
	loop.Status = StatusDeleted
	loop.NextRunAt = time.Time{}
	loop.UpdatedAt = s.now()
	return s.Repo.SaveLoop(loop)
}

func (s *Service) Runs(loopID string, limit int) ([]Run, error) {
	return s.Repo.ListRuns(loopID, limit)
}

func (s *Service) EnsureMemoryPaths() error {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.MemoryPaths == nil || s.MemoryPaths.Dir() == "" {
		return nil
	}
	items, err := s.Repo.ListLoops()
	if err != nil {
		return err
	}
	used := memoryPathSet(items)
	updates := make([]Loop, 0)
	for _, loop := range items {
		if !s.MemoryPaths.AssignMissing(&loop, used) {
			continue
		}
		loop.UpdatedAt = now
		used[loop.MemoryPath] = struct{}{}
		updates = append(updates, loop)
	}
	return s.Repo.SaveLoops(updates)
}

func (s *Service) StartDue(ctx context.Context) (bool, error) {
	now := s.now()
	s.mu.Lock()
	loop, run, ok, err := s.claimDueRunLocked(now)
	s.mu.Unlock()
	if err != nil || !ok {
		return ok, err
	}
	s.start(ctx, loop, run)
	return true, nil
}

func (s *Service) StartDueAll(ctx context.Context) (int, error) {
	started := 0
	for {
		if err := ctx.Err(); err != nil {
			return started, err
		}
		ok, err := s.StartDue(ctx)
		if err != nil || !ok {
			return started, err
		}
		started++
	}
}

// OnBoard reports whether loop publishes a widget to a board.
func (s *Service) OnBoard(loop Loop) bool {
	return s.artifactSurface(loop, Run{}) != ""
}

func (s *Service) RunNow(ctx context.Context, loopID string) (Run, error) {
	now := s.now()
	s.mu.Lock()
	loop, run, err := s.startManualRunLocked(loopID, now)
	s.mu.Unlock()
	if err != nil {
		return Run{}, err
	}
	s.start(ctx, loop, run)
	return run, nil
}

// RunTriggered hands an outside event, described by event, to a routine's
// run.
func (s *Service) RunTriggered(ctx context.Context, loopID, event string) (Run, error) {
	now := s.now()
	s.mu.Lock()
	loop, err := s.Repo.LoadLoop(loopID)
	var run Run
	start := false
	if err == nil {
		loop, run, start, err = s.fireLocked(loop, []string{event}, nil, now)
	}
	s.mu.Unlock()
	if err != nil {
		return Run{}, err
	}
	if start {
		s.start(ctx, loop, run)
	}
	return run, nil
}

// fireLocked hands events to loop: into its pending run when one waits, or
// into a new run. The new run starts now unless a begun run of the loop is
// still going, in which case it waits for that run to finish. taken records the
// observed messages behind events, saved with the run that now holds them.
// Events reach no run while the loop waits for the user to come back.
func (s *Service) fireLocked(loop Loop, events []string, taken []SeenEvent, now time.Time) (Loop, Run, bool, error) {
	if loop.Status == StatusDeleted {
		return Loop{}, Run{}, false, fmt.Errorf("loop %s has been deleted", loop.ID)
	}
	if s.heldAway(loop) {
		return Loop{}, Run{}, false, nil
	}
	runs, err := s.Repo.ActiveRuns(loop.ID)
	if err != nil {
		return Loop{}, Run{}, false, err
	}
	run := Run{ID: s.Repo.NewRunID(), LoopID: loop.ID, ScheduledFor: now, Status: RunStatusStarting, CreatedAt: now}
	pending := slices.IndexFunc(runs, func(run Run) bool { return run.Status == RunStatusStarting })
	if pending >= 0 {
		run = runs[pending]
	}
	for _, event := range events {
		run.Reason = foldReason(run.Reason, event)
	}
	for i := range taken {
		taken[i].RunID = run.ID
	}
	if len(runs) > 0 {
		if pending < 0 {
			s.ours[run.ID] = true
		}
		return Loop{}, run, false, s.Repo.SaveRunAndLoop(run, nil, taken...)
	}
	err = s.newRunLocked(loop, run, now, taken...)
	return loop, run, err == nil, err
}

// Begin starts a pending run's turn in threadID and returns the run's loop
// and what the turn sends. A run already begun, as one whose turn is started
// again after a restart, keeps its first start.
func (s *Service) Begin(runID, threadID string) (Loop, string, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.Repo.LoadRun(runID)
	if err != nil {
		return Loop{}, "", err
	}
	if run.Status == RunStatusStarting {
		run.ThreadID = threadID
		run.StartedAt = now
		run.Status = RunStatusRunning
		if err := s.saveRunAndLoopUpdateLocked(run, now); err != nil {
			return Loop{}, "", err
		}
	}
	loop, err := s.Repo.LoadLoop(run.LoopID)
	if err != nil {
		return Loop{}, "", err
	}
	thread := ThreadChild
	switch {
	case loop.BotID != "" && threadID == loop.BotID:
		thread = ThreadMain
	case s.thread(loop) == "":
		thread = ""
	}
	return loop, runPrompt(loop, run, now, thread), nil
}

// Closed reports whether a run has ended.
func (s *Service) Closed(runID string) (Run, bool, error) {
	run, err := s.Repo.LoadRun(runID)
	return run, err == nil && !active(run), err
}

// Finish closes a run that is still open, recording whether it delivered its
// outcome, then starts the run waiting behind it; a run already closed stays as
// it ended.
func (s *Service) Finish(runID, status, errText string, delivered bool) error {
	s.mu.Lock()
	loop, next, err := s.finishLocked(runID, status, errText, delivered)
	s.mu.Unlock()
	if err == nil && next.ID != "" {
		s.start(context.Background(), loop, next)
	}
	return err
}

func (s *Service) finishLocked(runID, status, errText string, delivered bool) (Loop, Run, error) {
	now := s.now()
	run, err := s.Repo.LoadRun(runID)
	if err != nil || !active(run) {
		return Loop{}, Run{}, err
	}
	delete(s.ours, run.ID)
	run.Status = normalizeRunStatus(status)
	run.Error = errText
	run.Delivered = delivered
	if run.Status == RunStatusOK && s.RunCheck != nil {
		loop, err := s.Repo.LoadLoop(run.LoopID)
		if err != nil {
			return Loop{}, Run{}, err
		}
		if err := s.RunCheck(loop, run); err != nil {
			run.Status, run.Error = RunStatusError, err.Error()
		}
	}
	run.FinishedAt = now
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	if err := s.saveRunAndLoopUpdateLocked(run, now); err != nil {
		return Loop{}, Run{}, err
	}
	return s.nextRunLocked(run.LoopID, now)
}

// nextRunLocked takes up the run waiting for loop's begun run to finish, if
// one waits and none is begun.
func (s *Service) nextRunLocked(loopID string, now time.Time) (Loop, Run, error) {
	runs, err := s.Repo.ActiveRuns(loopID)
	if err != nil || len(runs) != 1 || runs[0].Status != RunStatusStarting {
		return Loop{}, Run{}, err
	}
	loop, err := s.Repo.LoadLoop(loopID)
	if err != nil {
		return Loop{}, Run{}, err
	}
	return loop, runs[0], s.newRunLocked(loop, runs[0], now)
}

// Resume settles the runs a previous process left open, once its threads have
// resumed: held names the runs a thread still holds, queued or in a turn, and
// those go on. A begun run no thread holds is closed, since nothing will
// finish it; a pending one starts again unless it waits behind a begun run.
func (s *Service) Resume(ctx context.Context, held map[string]bool) error {
	s.mu.Lock()
	starts, err := s.resumeLocked(held)
	if err != nil {
		// The next try takes up the runs this one claimed.
		for _, start := range starts {
			delete(s.ours, start.run.ID)
		}
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, start := range starts {
		s.start(ctx, start.loop, start.run)
	}
	return nil
}

// dispatch is a run to hand its executor, with its loop as the run's prompt
// describes it.
type dispatch struct {
	loop Loop
	run  Run
}

func (s *Service) resumeLocked(held map[string]bool) ([]dispatch, error) {
	ids, err := s.Repo.ListActiveRunIDs()
	if err != nil {
		return nil, err
	}
	var pending []Run
	var starts []dispatch
	for _, id := range ids {
		if held[id] || s.ours[id] {
			continue
		}
		run, err := s.Repo.LoadRun(id)
		if err != nil {
			return starts, err
		}
		if run.Status == RunStatusStarting {
			pending = append(pending, run)
			continue
		}
		loop, next, err := s.finishLocked(id, RunStatusError, "Jaz stopped before this run finished.", false)
		if err != nil {
			return starts, err
		}
		if next.ID != "" {
			starts = append(starts, dispatch{loop, next})
		}
	}
	for _, run := range pending {
		runs, err := s.Repo.ActiveRuns(run.LoopID)
		if err != nil {
			return starts, err
		}
		if s.ours[run.ID] || slices.ContainsFunc(runs, begun) {
			continue
		}
		loop, err := s.Repo.LoadLoop(run.LoopID)
		if err != nil {
			return starts, err
		}
		s.ours[run.ID] = true
		starts = append(starts, dispatch{loop, run})
	}
	return starts, nil
}

func begun(run Run) bool {
	return run.Status == RunStatusRunning
}

func active(run Run) bool {
	return run.Status == RunStatusStarting || run.Status == RunStatusRunning
}

func (s *Service) start(ctx context.Context, loop Loop, run Run) {
	if s.Executor == nil {
		_ = s.Finish(run.ID, RunStatusError, "loop executor is not configured", false)
		return
	}
	extensions, err := s.RuntimePromptBuilder().For(loop, run, s.now())
	if err != nil {
		_ = s.Finish(run.ID, RunStatusError, err.Error(), false)
		return
	}
	go s.Executor.StartLoopRun(context.WithoutCancel(ctx), Execution{
		Loop:                   loop,
		Run:                    run,
		Thread:                 s.thread(loop),
		SystemPromptExtensions: extensions,
		ArtifactSurface:        s.artifactSurface(loop, run),
		Controller:             s,
	})
}

// thread is where a run of loop happens: ThreadMain or ThreadChild for a
// bot's routine, or "" for a run on a board or with no bot, which has a thread
// of its own.
func (s *Service) thread(loop Loop) string {
	if loop.BotID == "" || s.artifactSurface(loop, Run{}) != "" {
		return ""
	}
	return loop.Thread
}

func (s *Service) artifactSurface(loop Loop, run Run) string {
	if s.ArtifactSurface == nil {
		return ""
	}
	return strings.TrimSpace(s.ArtifactSurface(loop, run))
}

func SystemPromptExtensions(loop Loop, run, previous Run, now time.Time, extras ...string) promptmodule.Modules {
	return promptmodule.New(RunSystemPrompt(loop, run, previous, now, extras...))
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// claimDueRunLocked starts the next due loop's run. A slot older than
// missedAfter was missed while Jaz was not running, asleep or stopped: a
// loop that skips missed slots runs only for a slot still on time, and one
// that catches up covers every missed slot in one run.
func (s *Service) claimDueRunLocked(now time.Time) (Loop, Run, bool, error) {
	ids, err := s.Repo.ListDueLoopIDs(now)
	if err != nil {
		return Loop{}, Run{}, false, err
	}
	for _, id := range ids {
		loop, err := s.Repo.LoadLoop(id)
		if err != nil {
			return Loop{}, Run{}, false, err
		}
		scheduledFor := loop.NextRunAt
		if scheduledFor.IsZero() {
			continue
		}
		nextRun, err := NextRun(loop.Schedule, now)
		if err != nil {
			return Loop{}, Run{}, false, err
		}
		if s.heldAway(loop) {
			if err := s.skipRunLocked(loop, scheduledFor, nextRun, now, awayReason); err != nil {
				return Loop{}, Run{}, false, err
			}
			continue
		}
		if runs, err := s.Repo.ActiveRuns(loop.ID); err != nil {
			return Loop{}, Run{}, false, err
		} else if len(runs) > 0 {
			if err := s.skipRunLocked(loop, scheduledFor, nextRun, now, "previous loop run is still active"); err != nil {
				return Loop{}, Run{}, false, err
			}
			continue
		}
		missed, err := missedSlots(loop.Schedule, scheduledFor, now)
		if err != nil {
			return Loop{}, Run{}, false, err
		}
		run := Run{ID: s.Repo.NewRunID(), LoopID: loop.ID, ScheduledFor: scheduledFor, Status: RunStatusStarting, CreatedAt: now}
		switch {
		case missed.count == 0:
		case loop.WhenMissed == MissedCatchUp:
			lastOK, err := s.Repo.LastFinishedAt(loop.ID, RunStatusOK)
			if err != nil {
				return Loop{}, Run{}, false, err
			}
			run.Reason = catchUpReason(loop, missed, lastOK)
		case missed.due.IsZero():
			if err := s.skipRunLocked(loop, scheduledFor, nextRun, now, "Skipped: "+missedText(loop, missed)+"."); err != nil {
				return Loop{}, Run{}, false, err
			}
			continue
		default:
			run.ScheduledFor = missed.due
		}
		loop.NextRunAt = nextRun
		err = s.newRunLocked(loop, run, now)
		return loop, run, err == nil, err
	}
	return Loop{}, Run{}, false, nil
}

func (s *Service) startManualRunLocked(loopID string, now time.Time) (Loop, Run, error) {
	loop, err := s.Repo.LoadLoop(loopID)
	if err != nil {
		return Loop{}, Run{}, err
	}
	if loop.Status == StatusDeleted {
		return Loop{}, Run{}, fmt.Errorf("loop %s has been deleted", loop.ID)
	}
	if runs, err := s.Repo.ActiveRuns(loop.ID); err != nil {
		return Loop{}, Run{}, err
	} else if len(runs) > 0 {
		return Loop{}, Run{}, fmt.Errorf("loop %s already has an active run", loop.ID)
	}
	run := Run{ID: s.Repo.NewRunID(), LoopID: loop.ID, ScheduledFor: now, Status: RunStatusStarting, CreatedAt: now}
	return loop, run, s.newRunLocked(loop, run, now)
}

// newRunLocked saves run as loop's latest, with the messages it takes.
func (s *Service) newRunLocked(loop Loop, run Run, now time.Time, taken ...SeenEvent) error {
	loop.LastRunAt = now
	loop.LastRunID = run.ID
	loop.LastRunThreadID = ""
	loop.LastRunStatus = run.Status
	loop.LastError = ""
	loop.UpdatedAt = now
	if err := s.Repo.SaveRunAndLoop(run, &loop, taken...); err != nil {
		return err
	}
	s.ours[run.ID] = true
	return nil
}

func (s *Service) assignMemoryPathLocked(loop *Loop) error {
	if s.MemoryPaths == nil || s.MemoryPaths.Dir() == "" {
		return nil
	}
	items, err := s.Repo.ListLoops()
	if err != nil {
		return err
	}
	used := memoryPathSet(items)
	s.MemoryPaths.AssignMissing(loop, used)
	return nil
}

func (s *Service) skipRunLocked(loop Loop, scheduledFor, nextRun, now time.Time, reason string) error {
	run := Run{
		ID:           s.Repo.NewRunID(),
		LoopID:       loop.ID,
		ScheduledFor: scheduledFor,
		StartedAt:    now,
		FinishedAt:   now,
		Status:       RunStatusSkipped,
		Error:        reason,
		CreatedAt:    now,
	}
	loop.NextRunAt = nextRun
	loop.LastRunAt = now
	loop.LastRunID = run.ID
	loop.LastRunThreadID = ""
	loop.LastRunStatus = run.Status
	loop.LastError = reason
	loop.UpdatedAt = now
	return s.Repo.SaveRunAndLoop(run, &loop)
}

func (s *Service) saveRunAndLoopUpdateLocked(run Run, now time.Time) error {
	loop, err := s.loopUpdateForRunLocked(run, now)
	if err != nil {
		return err
	}
	return s.Repo.SaveRunAndLoop(run, loop)
}

func (s *Service) loopUpdateForRunLocked(run Run, now time.Time) (*Loop, error) {
	loop, err := s.Repo.LoadLoop(run.LoopID)
	if err != nil {
		return nil, err
	}
	if loop.LastRunID != run.ID {
		return nil, nil
	}
	loop.LastRunAt = firstTime(run.FinishedAt, run.StartedAt, run.CreatedAt, now)
	loop.LastRunThreadID = run.ThreadID
	loop.LastRunStatus = run.Status
	if run.Status == RunStatusOK {
		loop.LastError = ""
	} else {
		loop.LastError = run.Error
	}
	loop.UpdatedAt = now
	return &loop, nil
}

func RunSystemPrompt(loop Loop, run, previous Run, now time.Time, extras ...string) string {
	last := "none"
	if previous.ID != "" {
		var b strings.Builder
		fmt.Fprintf(&b, "id=%s status=%s delivered=%t", previous.ID, previous.Status, previous.Delivered)
		if at := firstTime(previous.FinishedAt, previous.StartedAt, previous.CreatedAt); !at.IsZero() {
			fmt.Fprintf(&b, " at=%s", at.Format(time.RFC3339))
		}
		if previous.ThreadID != "" {
			fmt.Fprintf(&b, " thread_id=%s", previous.ThreadID)
		}
		if previous.Error != "" {
			fmt.Fprintf(&b, " error=%q", previous.Error)
		}
		last = b.String()
	}
	return looprun.Render(looprun.Data{
		LoopName:     loop.Name,
		LoopID:       loop.ID,
		RunID:        run.ID,
		ScheduledFor: run.ScheduledFor.Format(time.RFC3339),
		Now:          now.UTC().Format(time.RFC3339),
		MemoryPath:   loop.MemoryPath,
		PreviousRun:  last,
		Extras:       extras,
	})
}

// settleDelays are the waits before each further try at settling the runs a
// previous process left open, as when the database is briefly unavailable;
// past the last, every try waits as long as the last.
var settleDelays = []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}

// StartScheduler settles the runs a previous process left open with settle,
// then starts due routines every tick. A failed try waits and tries again,
// for as long as it takes; until one succeeds no scheduled run starts, and
// each scheduled routine says why.
func StartScheduler(ctx context.Context, service *Service, tick time.Duration, settle func(context.Context) error) error {
	if service == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := service.settle(ctx, settle); err != nil {
		return err
	}
	if tick <= 0 {
		tick = 30 * time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		if err := service.GuardAway(); err != nil {
			service.Log.Error("away guard failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := service.StartDueAll(ctx); err != nil && ctx.Err() == nil {
				service.Log.Error("loop scheduler tick failed", "error", err)
			}
		}
	}
}

const unsettledReason = "Jaz could not check the runs it left open when it last stopped, so scheduled runs wait "

func (s *Service) settle(ctx context.Context, settle func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		err := settle(ctx)
		if err == nil || ctx.Err() != nil {
			s.setUnsettled("")
			return ctx.Err()
		}
		s.setUnsettled(unsettledReason + "while it tries again.")
		s.Log.Warn("checking the routine runs left open failed; trying again", "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(settleDelays[min(attempt, len(settleDelays)-1)]):
		}
	}
}

func (s *Service) setUnsettled(reason string) {
	s.mu.Lock()
	s.unsettled = reason
	s.mu.Unlock()
}

func normalizeRunStatus(status string) string {
	switch status {
	case RunStatusOK, RunStatusError, RunStatusCancelled, RunStatusSkipped:
		return status
	case "":
		return RunStatusOK
	default:
		return status
	}
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
