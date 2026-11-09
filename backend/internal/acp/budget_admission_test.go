package acp

import (
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBudgetAdmissionIncludesBotAndNestedWorkerSessions(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateSession(storage.CreateSession{Slug: "worker", ParentID: parent.ID, SourceType: storage.SourceBotWorker, SourceID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := store.CreateSession(storage.CreateSession{Slug: "nested", ParentID: worker.ID, SourceType: storage.SourceBotWorker, SourceID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []storage.Session{parent, worker, nested} {
		if err := store.UpdateSessionStatus(thread.ID, storage.StatusRunning, "", time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	policy := BudgetPolicy{MaxWorkersPerBot: 1}
	manager := NewManager(store, Config{Budget: &policy}, nil)
	request := SpawnRequest{SourceID: parent.ID}
	decision, err := manager.checkBudgetAdmission(request)
	if err == nil || decision.Decision != "queue" || decision.BotWorkers != 2 || decision.ActiveThreads != 3 {
		t.Fatalf("worker admission = %+v, %v", decision, err)
	}
	policy.MaxWorkersPerBot, policy.MaxActiveThreads = 0, 3
	decision, err = manager.checkBudgetAdmission(request)
	if err == nil || decision.Decision != "queue" || decision.ActiveThreads != 3 {
		t.Fatalf("global admission = %+v, %v", decision, err)
	}
	if err := store.SetArchived(worker.ID, true); err != nil {
		t.Fatal(err)
	}
	decision, err = manager.checkBudgetAdmission(request)
	if err != nil || decision.ActiveThreads != 1 || decision.BotWorkers != 0 {
		t.Fatalf("archived worker admission = %+v, %v", decision, err)
	}
}

var testBudgetPolicy = BudgetPolicy{MaxActiveThreads: 2, MaxWorkersPerBot: 1, SnapshotMaxAge: 15 * time.Minute, ReservePercent: 10}

func TestBudgetRequiresExplicitPolicyAndIgnoresExpiredWindows(t *testing.T) {
	manager := NewManager(nil, Config{}, nil)
	decision, err := manager.checkBudgetAdmission(SpawnRequest{})
	if err != nil || decision.Decision != "not_configured" {
		t.Fatalf("unconfigured policy = %+v, %v", decision, err)
	}
	now := time.Now().UTC()
	req := SpawnRequest{ModelProvider: "custom", SourceID: "bot"}
	snapshot := quotaSnapshot{Provider: "custom", ObservedAt: now, Windows: []quotaWindow{{Name: "old", UsedPercent: 100, ResetsAt: now.Add(-time.Second)}}}
	decision, err = decideBudgetAdmission(req, nil, snapshot, now, testBudgetPolicy)
	if err != nil || decision.Decision != "serial" || !strings.Contains(decision.Reason, "unknown") {
		t.Fatalf("expired window = %+v, %v", decision, err)
	}
	snapshot.Windows[0] = quotaWindow{Name: "current", UsedPercent: 95, ResetsAt: now.Add(time.Hour)}
	policy := testBudgetPolicy
	policy.ReservePercent = 3
	if decision, err = decideBudgetAdmission(req, nil, snapshot, now, policy); err != nil {
		t.Fatalf("configured reserve was ignored: %+v, %v", decision, err)
	}
}

func TestBotWorkerBudgetAdmissionUsesLiveThreadsAndQuotaSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	request := SpawnRequest{ACPAgent: "opencode", ModelProvider: "opencode-go", Model: "deepseek-v4-flash", SourceType: storage.SourceBotWorker, SourceID: "bot-1"}
	available := quotaSnapshot{
		Provider: "opencode-go", Source: "OpenCode Console", ObservedAt: now.Add(-time.Minute),
		Windows: []quotaWindow{{Name: "weekly", UsedPercent: 70, ResetsAt: now.Add(time.Hour)}},
	}
	activeParent := storage.Session{ID: "bot-1", Status: storage.StatusRunning, SourceType: storage.SourceBot}

	decision, err := decideBudgetAdmission(request, []storage.Session{activeParent}, available, now, testBudgetPolicy)
	if err != nil || decision.Decision != "serial" || decision.QuotaSource != "OpenCode Console" {
		t.Fatalf("available serial admission = %+v, %v", decision, err)
	}

	decision, err = decideBudgetAdmission(request, []storage.Session{
		activeParent,
		{ID: "worker-1", Status: storage.StatusRunning, SourceType: storage.SourceBotWorker, SourceID: "bot-1"},
	}, available, now, testBudgetPolicy)
	if err == nil || decision.Decision != "queue" || !strings.Contains(err.Error(), "active workers") {
		t.Fatalf("same-bot worker admission = %+v, %v", decision, err)
	}

	exhausted := available
	exhausted.Windows[0].UsedPercent = 100
	decision, err = decideBudgetAdmission(request, []storage.Session{activeParent}, exhausted, now, testBudgetPolicy)
	if err == nil || decision.Decision != "wait" || !strings.Contains(err.Error(), "exhausted quota window") {
		t.Fatalf("exhausted quota admission = %+v, %v", decision, err)
	}

	low := available
	low.Windows[0].UsedPercent = 95
	decision, err = decideBudgetAdmission(request, []storage.Session{activeParent}, low, now, testBudgetPolicy)
	if err == nil || decision.Decision != "conserve" {
		t.Fatalf("reserve admission = %+v, %v", decision, err)
	}

	stale := available
	stale.ObservedAt = now.Add(-testBudgetPolicy.SnapshotMaxAge - time.Second)
	decision, err = decideBudgetAdmission(request, []storage.Session{activeParent}, stale, now, testBudgetPolicy)
	if err != nil || decision.Decision != "serial" || !strings.Contains(decision.Reason, "stale") {
		t.Fatalf("stale quota serial admission = %+v, %v", decision, err)
	}
	stale.Windows[0].UsedPercent = 100
	decision, err = decideBudgetAdmission(request, []storage.Session{activeParent}, stale, now, testBudgetPolicy)
	if err != nil || decision.Decision != "serial" || !strings.Contains(decision.Reason, "stale") {
		t.Fatalf("stale exhausted snapshot must not block = %+v, %v", decision, err)
	}
}

func TestBotWorkerBudgetAdmissionCountsAllActiveThreads(t *testing.T) {
	now := time.Now().UTC()
	request := SpawnRequest{ACPAgent: "codex", Model: "gpt", SourceType: storage.SourceBotWorker, SourceID: "bot"}
	snapshot := quotaSnapshot{Provider: "codex", Source: "cabinet", ObservedAt: now, Windows: []quotaWindow{{Name: "week", UsedPercent: 5, ResetsAt: now.Add(time.Hour)}}}
	sessions := []storage.Session{
		{ID: "parent", Status: storage.StatusRunning, SourceType: storage.SourceBot},
		{ID: "unrelated", Status: storage.StatusRunning},
	}
	decision, err := decideBudgetAdmission(request, sessions, snapshot, now, testBudgetPolicy)
	if err == nil || decision.Decision != "queue" || decision.ActiveThreads != 2 {
		t.Fatalf("global active-thread gate = %+v, %v", decision, err)
	}
}
