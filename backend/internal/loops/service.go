package loops

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/templates/looprun"
)

type Service struct {
	Repo        Repository
	Executor    Executor
	Log         *log.Logger
	Now         func() time.Time
	MemoryPaths *MemoryPaths
	// PromptExtra appends capability sections (e.g. widget instructions) to the
	// metadata prompt of each run.
	PromptExtra     func(Loop, Run) string
	ArtifactSurface func(Loop, Run) string
	// RunFinished fires after a run reaches a terminal status.
	RunFinished func(Loop, Run)
	mu          sync.Mutex
}

type Option func(*Service)

func WithMemoryPaths(paths *MemoryPaths) Option {
	return func(s *Service) {
		s.MemoryPaths = paths
	}
}

func WithPromptExtra(extra func(Loop, Run) string) Option {
	return func(s *Service) {
		s.PromptExtra = extra
	}
}

func WithArtifactSurface(surface func(Loop, Run) string) Option {
	return func(s *Service) {
		s.ArtifactSurface = surface
	}
}

func WithRunFinished(hook func(Loop, Run)) Option {
	return func(s *Service) {
		s.RunFinished = hook
	}
}

func NewService(repo Repository, executor Executor, logger *log.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = log.Default()
	}
	service := &Service{Repo: repo, Executor: executor, Log: logger.WithPrefix("loops")}
	for _, opt := range opts {
		opt(service)
	}
	return service
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
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.assignMemoryPathLocked(&loop); err != nil {
		return Loop{}, err
	}
	return loop, s.Repo.SaveLoop(loop)
}

func (s *Service) Load(id string) (Loop, error) {
	return s.Repo.LoadLoop(id)
}

func (s *Service) List() ([]Loop, error) {
	return s.Repo.ListLoops()
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
	next.UpdatedAt = now
	if err := s.Repo.SaveLoop(next); err != nil {
		return Loop{}, err
	}
	return s.Repo.LoadLoop(id)
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

func (s *Service) AdvanceMissed() error {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.resetStaleRunsLocked(now); err != nil {
		return err
	}
	ids, err := s.Repo.ListDueLoopIDs(now)
	if err != nil {
		return err
	}
	updates := make([]Loop, 0, len(ids))
	for _, id := range ids {
		loop, err := s.Repo.LoadLoop(id)
		if err != nil {
			return err
		}
		nextRun, err := NextRun(loop.Schedule, now)
		if err != nil {
			return err
		}
		loop.NextRunAt = nextRun
		loop.UpdatedAt = now
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
	s.start(ctx, loop, run, now)
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

func (s *Service) RunNow(ctx context.Context, loopID string) (Run, error) {
	now := s.now()
	s.mu.Lock()
	loop, run, err := s.startManualRunLocked(loopID, now)
	s.mu.Unlock()
	if err != nil {
		return Run{}, err
	}
	s.start(ctx, loop, run, now)
	return run, nil
}

func (s *Service) MarkRunning(runID, threadID string) error {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.Repo.LoadRun(runID)
	if err != nil {
		return err
	}
	run.ThreadID = threadID
	run.StartedAt = now
	run.Status = RunStatusRunning
	return s.saveRunAndLoopUpdateLocked(run, now)
}

func (s *Service) Finish(runID, status, errText string) error {
	now := s.now()
	s.mu.Lock()
	run, err := s.Repo.LoadRun(runID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	run.Status = normalizeRunStatus(status)
	run.Error = errText
	run.FinishedAt = now
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	err = s.saveRunAndLoopUpdateLocked(run, now)
	s.mu.Unlock()
	if err == nil {
		s.notifyRunFinished(run)
	}
	return err
}

func (s *Service) FinishThread(threadID, status, errText string) (Run, bool, error) {
	now := s.now()
	s.mu.Lock()
	run, ok, err := s.Repo.LoadRunByThread(threadID)
	if err != nil || !ok {
		s.mu.Unlock()
		return Run{}, ok, err
	}
	if run.Status != RunStatusStarting && run.Status != RunStatusRunning {
		s.mu.Unlock()
		return run, true, nil
	}
	run.Status = normalizeRunStatus(status)
	run.Error = errText
	run.FinishedAt = now
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	if err := s.saveRunAndLoopUpdateLocked(run, now); err != nil {
		s.mu.Unlock()
		return Run{}, false, err
	}
	s.mu.Unlock()
	s.notifyRunFinished(run)
	return run, true, nil
}

func (s *Service) notifyRunFinished(run Run) {
	if s.RunFinished == nil {
		return
	}
	loop, err := s.Repo.LoadLoop(run.LoopID)
	if err != nil {
		return
	}
	go s.RunFinished(loop, run)
}

func (s *Service) start(ctx context.Context, loop Loop, run Run, now time.Time) {
	if s.Executor == nil {
		_ = s.Finish(run.ID, RunStatusError, "loop executor is not configured")
		return
	}
	var extras []string
	if s.PromptExtra != nil {
		if extra := strings.TrimSpace(s.PromptExtra(loop, run)); extra != "" {
			extras = append(extras, extra)
		}
	}
	artifactSurface := ""
	if s.ArtifactSurface != nil {
		artifactSurface = strings.TrimSpace(s.ArtifactSurface(loop, run))
	}
	prompt := RunPrompt(loop, run, now, extras...)
	go s.Executor.StartLoopRun(context.WithoutCancel(ctx), Execution{
		Loop:            loop,
		Run:             run,
		Prompt:          prompt,
		ArtifactSurface: artifactSurface,
		Controller:      s,
	})
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

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
		if active, err := s.Repo.HasActiveRun(loop.ID); err != nil {
			return Loop{}, Run{}, false, err
		} else if active {
			if err := s.skipRunLocked(loop, scheduledFor, nextRun, now, "previous loop run is still active"); err != nil {
				return Loop{}, Run{}, false, err
			}
			continue
		}
		promptLoop := loop
		run := Run{
			ID:           s.Repo.NewRunID(),
			LoopID:       loop.ID,
			ScheduledFor: scheduledFor,
			Status:       RunStatusStarting,
			CreatedAt:    now,
		}
		loop.NextRunAt = nextRun
		loop.LastRunAt = now
		loop.LastRunID = run.ID
		loop.LastRunThreadID = ""
		loop.LastRunStatus = run.Status
		loop.LastError = ""
		loop.UpdatedAt = now
		if err := s.Repo.SaveLoopAndRun(loop, run); err != nil {
			return Loop{}, Run{}, false, err
		}
		return promptLoop, run, true, nil
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
	if active, err := s.Repo.HasActiveRun(loop.ID); err != nil {
		return Loop{}, Run{}, err
	} else if active {
		return Loop{}, Run{}, fmt.Errorf("loop %s already has an active run", loop.ID)
	}
	promptLoop := loop
	run := Run{
		ID:           s.Repo.NewRunID(),
		LoopID:       loop.ID,
		ScheduledFor: now,
		Status:       RunStatusStarting,
		CreatedAt:    now,
	}
	loop.LastRunAt = now
	loop.LastRunID = run.ID
	loop.LastRunThreadID = ""
	loop.LastRunStatus = run.Status
	loop.LastError = ""
	loop.UpdatedAt = now
	if err := s.Repo.SaveLoopAndRun(loop, run); err != nil {
		return Loop{}, Run{}, err
	}
	return promptLoop, run, nil
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
	return s.Repo.SaveLoopAndRun(loop, run)
}

func (s *Service) resetStaleRunsLocked(now time.Time) error {
	ids, err := s.Repo.ListActiveRunIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		run, err := s.Repo.LoadRun(id)
		if err != nil {
			return err
		}
		run.Status = RunStatusError
		run.Error = "Server restarted while this loop run was still active."
		run.FinishedAt = now
		if run.StartedAt.IsZero() {
			run.StartedAt = now
		}
		if err := s.saveRunAndLoopUpdateLocked(run, now); err != nil {
			return err
		}
	}
	return nil
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

// RunPrompt renders the full prompt for a run via the looprun template:
// context and rules first, capability extras (widget instructions, …) next,
// and the user's task last so it carries the emphasis.
func RunPrompt(loop Loop, run Run, now time.Time, extras ...string) string {
	previous := "none"
	if loop.LastRunID != "" {
		var b strings.Builder
		fmt.Fprintf(&b, "id=%s status=%s", loop.LastRunID, loop.LastRunStatus)
		if !loop.LastRunAt.IsZero() {
			fmt.Fprintf(&b, " at=%s", loop.LastRunAt.Format(time.RFC3339))
		}
		if loop.LastRunThreadID != "" {
			fmt.Fprintf(&b, " thread_id=%s", loop.LastRunThreadID)
		}
		if loop.LastError != "" {
			fmt.Fprintf(&b, " error=%q", loop.LastError)
		}
		previous = b.String()
	}
	prompt, err := looprun.Render(looprun.Data{
		LoopName:     loop.Name,
		LoopID:       loop.ID,
		RunID:        run.ID,
		ScheduledFor: run.ScheduledFor.Format(time.RFC3339),
		Now:          now.UTC().Format(time.RFC3339),
		MemoryPath:   loop.MemoryPath,
		PreviousRun:  previous,
		Extras:       extras,
		Prompt:       loop.Prompt,
	})
	if err != nil {
		// The template is embedded and parse-checked at init; execution
		// cannot realistically fail, but the run must never lose its task.
		return loop.Prompt
	}
	return prompt
}

func StartScheduler(ctx context.Context, service *Service, tick time.Duration) error {
	if service == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	if tick <= 0 {
		tick = 30 * time.Second
	}
	if err := service.AdvanceMissed(); err != nil {
		return err
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
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
