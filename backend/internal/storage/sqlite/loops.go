package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/loopdb"
)

func (s *Store) NewLoopID() string {
	return "loop-" + s.NewSessionID()
}

func (s *Store) NewRunID() string {
	return "run-" + s.NewSessionID()
}

func (s *Store) LoadLoop(id string) (loops.Loop, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := loopdb.New(s.db).GetLoop(context.Background(), id)
	if err != nil {
		return loops.Loop{}, err
	}
	return loopFromDB(row), nil
}

func (s *Store) ListLoops() ([]loops.Loop, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := loopdb.New(s.db).ListLoops(context.Background(), loops.StatusDeleted)
	if err != nil {
		return nil, err
	}
	out := make([]loops.Loop, 0, len(rows))
	for _, row := range rows {
		out = append(out, loopFromDB(row))
	}
	return out, nil
}

func (s *Store) ListRuns(loopID string, limit int) ([]loops.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	rows, err := loopdb.New(s.db).ListRunsByLoop(context.Background(), loopdb.ListRunsByLoopParams{
		LoopID: loopID,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]loops.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, runFromDB(row))
	}
	return out, nil
}

func (s *Store) LoadRun(id string) (loops.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := loopdb.New(s.db).GetRun(context.Background(), id)
	if err != nil {
		return loops.Run{}, err
	}
	return runFromDB(row), nil
}

func (s *Store) LoadRunByThread(threadID string) (loops.Run, bool, error) {
	if threadID == "" {
		return loops.Run{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := loopdb.New(s.db).GetLatestRunByThread(context.Background(), nullDBString(threadID))
	if err == sql.ErrNoRows {
		return loops.Run{}, false, nil
	}
	if err != nil {
		return loops.Run{}, false, err
	}
	return runFromDB(row), true, nil
}

func (s *Store) ListDueLoopIDs(now time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return loopdb.New(s.db).ListDueLoopIDs(context.Background(), loopdb.ListDueLoopIDsParams{
		ActiveStatus: loops.StatusActive,
		NowMs:        loopTimeToMs(now),
	})
}

func (s *Store) ListActiveRunIDs() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return loopdb.New(s.db).ListActiveRunIDs(context.Background(), loopdb.ListActiveRunIDsParams{
		StartingStatus: loops.RunStatusStarting,
		RunningStatus:  loops.RunStatusRunning,
	})
}

func (s *Store) HasActiveRun(loopID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := loopdb.New(s.db).GetActiveRunIDForLoop(context.Background(), loopdb.GetActiveRunIDForLoopParams{
		LoopID:         loopID,
		StartingStatus: loops.RunStatusStarting,
		RunningStatus:  loops.RunStatusRunning,
	})
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) SaveLoop(loop loops.Loop) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return upsertLoop(context.Background(), loopdb.New(s.db), loop)
}

func (s *Store) SaveRun(run loops.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return upsertRun(context.Background(), loopdb.New(s.db), run)
}

func (s *Store) SaveLoops(items []loops.Loop) error {
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withLoopTxLocked(func(q *loopdb.Queries) error {
		for _, loop := range items {
			if err := upsertLoop(context.Background(), q, loop); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) SaveLoopAndRun(loop loops.Loop, run loops.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withLoopTxLocked(func(q *loopdb.Queries) error {
		if err := upsertRun(context.Background(), q, run); err != nil {
			return err
		}
		return upsertLoop(context.Background(), q, loop)
	})
}

func (s *Store) SaveRunAndLoop(run loops.Run, loop *loops.Loop) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withLoopTxLocked(func(q *loopdb.Queries) error {
		if err := upsertRun(context.Background(), q, run); err != nil {
			return err
		}
		if loop == nil {
			return nil
		}
		return upsertLoop(context.Background(), q, *loop)
	})
}

func (s *Store) withLoopTxLocked(fn func(*loopdb.Queries) error) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(loopdb.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertLoop(ctx context.Context, q *loopdb.Queries, loop loops.Loop) error {
	return q.UpsertLoop(ctx, loopdb.UpsertLoopParams{
		ID:              loop.ID,
		Name:            loop.Name,
		Prompt:          loop.Prompt,
		ScheduleKind:    loop.Schedule.Kind,
		ScheduleExpr:    loop.Schedule.Expr,
		Timezone:        loop.Schedule.Timezone,
		Status:          loop.Status,
		Runtime:         loop.Runtime,
		AcpAgent:        nullDBString(loop.ACPAgent),
		ModelProvider:   loop.ModelProvider,
		Model:           loop.Model,
		ReasoningEffort: loop.ReasoningEffort,
		Directory:       loop.Directory,
		MemoryPath:      loop.MemoryPath,
		NextRunAtMs:     loopTimeToMs(loop.NextRunAt),
		LastRunAtMs:     loopTimeToMs(loop.LastRunAt),
		LastRunID:       nullDBString(loop.LastRunID),
		LastRunThreadID: nullDBString(loop.LastRunThreadID),
		LastRunStatus:   nullDBString(loop.LastRunStatus),
		LastError:       nullDBString(loop.LastError),
		CreatedAtMs:     loopTimeToMs(loop.CreatedAt),
		UpdatedAtMs:     loopTimeToMs(loop.UpdatedAt),
	})
}

func upsertRun(ctx context.Context, q *loopdb.Queries, run loops.Run) error {
	return q.UpsertRun(ctx, loopdb.UpsertRunParams{
		ID:             run.ID,
		LoopID:         run.LoopID,
		ThreadID:       nullDBString(run.ThreadID),
		ScheduledForMs: loopTimeToMs(run.ScheduledFor),
		StartedAtMs:    loopTimeToMs(run.StartedAt),
		FinishedAtMs:   loopTimeToMs(run.FinishedAt),
		Status:         run.Status,
		Error:          nullDBString(run.Error),
		CreatedAtMs:    loopTimeToMs(run.CreatedAt),
	})
}

func loopFromDB(row loopdb.Loop) loops.Loop {
	return loops.Loop{
		ID:              row.ID,
		Name:            row.Name,
		Prompt:          row.Prompt,
		Schedule:        loops.Schedule{Kind: row.ScheduleKind, Expr: row.ScheduleExpr, Timezone: row.Timezone},
		Status:          row.Status,
		Runtime:         row.Runtime,
		ACPAgent:        row.AcpAgent.String,
		ModelProvider:   row.ModelProvider,
		Model:           row.Model,
		ReasoningEffort: row.ReasoningEffort,
		Directory:       row.Directory,
		MemoryPath:      row.MemoryPath,
		NextRunAt:       msToTime(row.NextRunAtMs),
		LastRunAt:       msToTime(row.LastRunAtMs),
		LastRunID:       row.LastRunID.String,
		LastRunThreadID: row.LastRunThreadID.String,
		LastRunStatus:   row.LastRunStatus.String,
		LastError:       row.LastError.String,
		CreatedAt:       msToTime(row.CreatedAtMs),
		UpdatedAt:       msToTime(row.UpdatedAtMs),
	}
}

func runFromDB(row loopdb.LoopRun) loops.Run {
	return loops.Run{
		ID:           row.ID,
		LoopID:       row.LoopID,
		ThreadID:     row.ThreadID.String,
		ScheduledFor: msToTime(row.ScheduledForMs),
		StartedAt:    msToTime(row.StartedAtMs),
		FinishedAt:   msToTime(row.FinishedAtMs),
		Status:       row.Status,
		Error:        row.Error.String,
		CreatedAt:    msToTime(row.CreatedAtMs),
	}
}

func loopTimeToMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}
