package acp

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

func TestNativeCodexUsageAcrossActivities(t *testing.T) {
	raw, err := os.ReadFile("testdata/codex-usage-0.153.4.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Prompts []struct {
			Updates  []json.RawMessage `json:"updates"`
			Response json.RawMessage   `json:"response"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"", storage.SourceMemorySearch, storage.SourceMemorySource, storage.SourceLoopRun} {
		t.Run(source, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			session, err := store.CreateSession(storage.CreateSession{Slug: "native-usage", Runtime: storage.RuntimeACP, SourceType: source})
			if err != nil {
				t.Fatal(err)
			}
			manager := NewManager(store, Config{}, nil)
			manager.Events = sessionevents.New()
			job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentCodex, ACPSession: "native-session"}}
			manager.jobsByID[session.ID] = job
			manager.jobsByACP[job.ACPSession] = job
			for _, prompt := range fixture.Prompts {
				job.startTurn(CompletionInline, false, false)
				for _, update := range prompt.Updates {
					manager.applyUpdate(job.ACPSession, update)
					manager.applyUpdate(job.ACPSession, update)
				}
				manager.recordRawUsage(job, prompt.Response)
			}
			loaded, err := store.LoadSession(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Usage.InputTokens != 68_962 || loaded.Usage.CachedInputTokens != 58_368 || loaded.Usage.OutputTokens != 169 || loaded.Usage.TotalTokens != 69_131 {
				t.Fatalf("native usage = %#v", loaded.Usage)
			}
			days, err := usagecore.NewService(store).Daily(usagecore.DailyQuery{Days: 1, Location: time.UTC})
			if err != nil {
				t.Fatal(err)
			}
			if days[0].Usage.InputOutputTokens() != 10_763 || len(days[0].Categories) != 1 || days[0].Categories[0].Usage.InputOutputTokens() != 10_763 {
				t.Fatalf("activity totals = %#v", days[0])
			}
		})
	}
}

func TestConcurrentCodexUsageScopesPreserveMainContext(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "scoped-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: session.ID}}
	main := json.RawMessage(`{"sessionUpdate":"usage_update","used":500,"size":1000,"_meta":{"usageId":"main-1","usage":{"inputTokens":100,"outputTokens":20,"totalTokens":120}}}`)
	side := json.RawMessage(`{"sessionUpdate":"usage_update","used":900,"size":2000,"_meta":{"codex":{"sideChat":{"id":"side"}},"usageId":"side-1","usage":{"inputTokens":200,"outputTokens":40,"totalTokens":240}}}`)
	manager.recordRawUsage(job, main)
	manager.recordRawUsage(job, side)
	job.startTurn(CompletionInline, false, false)
	manager.recordRawUsage(job, side)
	manager.recordRawUsage(job, json.RawMessage(`{"usage":{"inputTokens":200,"outputTokens":40,"totalTokens":240},"_meta":{"codex":{"sideChat":{"id":"side"}},"usageId":"side-1"}}`))
	manager.recordRawUsage(job, main)
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 300 || loaded.Usage.OutputTokens != 60 || loaded.Usage.TotalTokens != 360 || loaded.Usage.ContextTokens != 500 || loaded.Usage.ContextWindowTokens != 1000 {
		t.Fatalf("scoped usage = %#v", loaded.Usage)
	}
}
