package acp

import (
	"encoding/json"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestUsageFromRawReadsOpenRouterExtras(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"usage": {
			"prompt_tokens": 128,
			"completion_tokens": 16,
			"total_tokens": 144,
			"cache_read_input_tokens": 96,
			"completion_tokens_details": {"reasoning_tokens": 4}
		}
	}`))
	if usage.InputTokens != 128 || usage.CachedInputTokens != 96 || usage.OutputTokens != 16 ||
		usage.ReasoningOutputTokens != 4 || usage.TotalTokens != 144 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawReadsTokenCacheShape(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"tokens": {
			"input": 200,
			"output": 30,
			"reasoning": 8,
			"cache": {"read": 160, "write": 20}
		}
	}`))
	if usage.InputTokens != 200 || usage.CachedInputTokens != 160 || usage.OutputTokens != 30 ||
		usage.ReasoningOutputTokens != 8 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestACPUsagePersistsAtTurnFinish(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	manager.recordUsage(job, usageFromRaw(json.RawMessage(`{
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 10,
			"cache_read_input_tokens": 60
		}
	}`)))
	manager.recordUsage(job, usageFromRaw(json.RawMessage(`{
		"usage": {
			"prompt_tokens": 120,
			"completion_tokens": 15,
			"total_tokens": 135
		}
	}`)))
	manager.persistUsage(job)

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 120 || loaded.Usage.CachedInputTokens != 60 ||
		loaded.Usage.OutputTokens != 15 || loaded.Usage.TotalTokens != 135 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
}

func TestACPUsagePersistsAtTurnFinishToSQLite(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp-sqlite-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	manager.recordUsage(job, usageFromRaw(json.RawMessage(`{
		"tokens": {
			"input": 200,
			"output": 30,
			"reasoning": 8,
			"cache": {"read": 160}
		}
	}`)))
	manager.persistUsage(job)

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 200 || loaded.Usage.CachedInputTokens != 160 ||
		loaded.Usage.OutputTokens != 30 || loaded.Usage.ReasoningOutputTokens != 8 ||
		loaded.Usage.TotalTokens != 230 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
}
