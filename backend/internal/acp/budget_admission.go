package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type BudgetPolicy struct {
	MaxActiveThreads int
	MaxWorkersPerBot int
	SnapshotPath     string
	SnapshotMaxAge   time.Duration
	ReservePercent   float64
}

type BudgetDecision struct {
	Decision       string    `json:"decision"`
	Reason         string    `json:"reason"`
	Provider       string    `json:"provider,omitempty"`
	Model          string    `json:"model,omitempty"`
	QuotaSource    string    `json:"quota_source,omitempty"`
	ObservedAt     time.Time `json:"observed_at,omitzero"`
	QuotaAgeSecond int64     `json:"quota_age_seconds,omitempty"`
	ActiveThreads  int       `json:"active_threads"`
	BotWorkers     int       `json:"active_workers_for_bot"`
}

type quotaSnapshot struct {
	Provider   string        `json:"provider"`
	Source     string        `json:"source"`
	ObservedAt time.Time     `json:"observed_at"`
	Windows    []quotaWindow `json:"windows"`
}

type quotaWindow struct {
	Name        string    `json:"name"`
	UsedPercent float64   `json:"used_percent"`
	ResetsAt    time.Time `json:"resets_at"`
}

// StartBudgeted is the mandatory admission path for threads created by an
// agent. It checks live sessions and the persisted quota observation while
// holding a process-wide admission lock, then keeps the decision with the
// created thread so the caller can explain it.
func (m *Manager) StartBudgeted(ctx context.Context, req SpawnRequest, first storage.QueuedMessage) (Job, BudgetDecision, error) {
	req, err := m.botThread(req)
	if err != nil {
		return Job{}, BudgetDecision{}, err
	}
	if req.SourceType != storage.SourceBotWorker {
		job, err := m.Start(ctx, req, first)
		return job, BudgetDecision{Decision: "not_applicable", Reason: "This thread is not a bot worker"}, err
	}
	effective, cfg, _, err := m.spawnConfig(req)
	if err != nil {
		return Job{}, BudgetDecision{}, err
	}
	budgetReq := req
	budgetReq.ModelProvider = sessionModelProvider(effective.ACPAgent, cfg)
	budgetReq.Model = cfg.Model

	m.budgetMu.Lock()
	defer m.budgetMu.Unlock()
	decision, err := m.checkBudgetAdmission(budgetReq)
	if err != nil {
		return Job{}, decision, fmt.Errorf("agent budget preflight: decision=%s provider=%s model=%s active=%d workers_for_bot=%d source=%q observed_at=%s: %w",
			decision.Decision, decision.Provider, decision.Model, decision.ActiveThreads, decision.BotWorkers,
			decision.QuotaSource, decision.ObservedAt.Format(time.RFC3339), err)
	}
	job, err := m.Start(ctx, req, first)
	return job, decision, err
}

func (m *Manager) checkBudgetAdmission(req SpawnRequest) (BudgetDecision, error) {
	if m.cfg.Budget == nil {
		return BudgetDecision{Decision: "not_configured", Reason: "No admission policy is configured"}, nil
	}
	policy := *m.cfg.Budget
	if policy.MaxActiveThreads < 0 || policy.MaxWorkersPerBot < 0 || policy.ReservePercent < 0 || policy.ReservePercent > 100 || (policy.SnapshotPath != "" && policy.SnapshotMaxAge <= 0) {
		return BudgetDecision{}, errors.New("invalid budget policy")
	}
	sessions, err := m.store.ListSessions(storage.SessionFilter{IncludeChildren: true, IncludeSourced: true})
	if err != nil {
		return BudgetDecision{}, fmt.Errorf("agent budget preflight: read active sessions: %w", err)
	}
	path := policy.SnapshotPath
	if path != "" && !filepath.IsAbs(path) {
		path = filepath.Join(m.cfg.Root, path)
	}
	data, _ := os.ReadFile(path)
	var snapshot quotaSnapshot
	_ = json.Unmarshal(data, &snapshot)
	return decideBudgetAdmission(req, sessions, snapshot, time.Now(), policy)
}

func decideBudgetAdmission(req SpawnRequest, sessions []storage.Session, snapshot quotaSnapshot, now time.Time, policy BudgetPolicy) (BudgetDecision, error) {
	decision := BudgetDecision{Decision: "serial", Reason: "Quota is unknown; only configured concurrency limits apply", Provider: req.ModelProvider, Model: req.Model}
	for _, session := range sessions {
		if session.Archived || session.Status != storage.StatusRunning {
			continue
		}
		decision.ActiveThreads++
		if session.SourceType == storage.SourceBotWorker && session.SourceID == req.SourceID {
			decision.BotWorkers++
		}
	}
	if policy.MaxWorkersPerBot > 0 && decision.BotWorkers >= policy.MaxWorkersPerBot {
		decision.Decision, decision.Reason = "queue", "This bot already has the configured number of active workers; do not create a duplicate"
		return decision, errors.New(decision.Reason)
	}
	if policy.MaxActiveThreads > 0 && decision.ActiveThreads >= policy.MaxActiveThreads {
		decision.Decision, decision.Reason = "queue", "The configured active-thread limit has been reached; retry this task later"
		return decision, errors.New(decision.Reason)
	}

	if snapshot.Provider == "" || snapshot.ObservedAt.IsZero() || len(snapshot.Windows) == 0 {
		return decision, nil
	}
	decision.QuotaSource, decision.ObservedAt = snapshot.Source, snapshot.ObservedAt
	decision.QuotaAgeSecond = int64(now.Sub(snapshot.ObservedAt).Seconds())
	if snapshot.Provider != budgetProvider(req) {
		decision.Reason = "The quota snapshot belongs to another provider; only configured concurrency limits apply"
		return decision, nil
	}
	if policy.SnapshotMaxAge <= 0 || now.Sub(snapshot.ObservedAt) > policy.SnapshotMaxAge || snapshot.ObservedAt.Sub(now) > time.Minute {
		decision.Reason = "The quota snapshot is stale or has an invalid timestamp; only configured concurrency limits apply"
		return decision, nil
	}
	var blocked []string
	var minimumRemaining = 100.0
	activeWindows := 0
	for _, window := range snapshot.Windows {
		if !window.ResetsAt.After(now) {
			continue
		}
		if window.UsedPercent < 0 || window.UsedPercent > 100 {
			decision.Reason = "The quota snapshot contains an invalid usage percentage"
			return decision, nil
		}
		activeWindows++
		if window.ResetsAt.After(now) && window.UsedPercent >= 100 {
			blocked = append(blocked, window.Name)
		}
		if remaining := 100 - window.UsedPercent; remaining < minimumRemaining {
			minimumRemaining = remaining
		}
	}
	if activeWindows == 0 {
		decision.Reason = "The snapshot windows have expired; current quota is unknown"
		return decision, nil
	}
	if len(blocked) > 0 {
		decision.Decision, decision.Reason = "wait", "The snapshot reports an exhausted quota window ("+strings.Join(blocked, ", ")+"); check the reset and refresh the snapshot"
		return decision, errors.New(decision.Reason)
	}
	if minimumRemaining <= policy.ReservePercent {
		decision.Decision, decision.Reason = "conserve", fmt.Sprintf("Remaining quota is at or below the configured %.0f%% reserve; this worker was not started", policy.ReservePercent)
		return decision, errors.New(decision.Reason)
	}
	decision.Reason = "A fresh matching quota snapshot permits this worker"
	return decision, nil
}

func budgetProvider(req SpawnRequest) string {
	if req.ModelProvider != "" {
		return req.ModelProvider
	}
	return req.ACPAgent
}
