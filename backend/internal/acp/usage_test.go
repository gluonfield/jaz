package acp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

func TestUsageFromRawReadsOpenRouterExtras(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"usage": {
			"prompt_tokens": 128,
			"completion_tokens": 16,
			"total_tokens": 144,
			"prompt_tokens_details": {"cached_tokens": 90, "cache_write_tokens": 6},
			"completion_tokens_details": {"reasoning_tokens": 4}
		}
	}`))
	// prompt_tokens counts cached reads inclusively; normalized to disjoint.
	if usage.InputTokens != 32 || usage.CachedInputTokens != 90 || usage.CachedWriteTokens != 6 || usage.OutputTokens != 16 ||
		usage.ReasoningOutputTokens != 4 || usage.TotalTokens != 144 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawReadsGrokPromptMeta(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"stopReason": "end_turn",
		"_meta": {
			"totalTokens": 14112,
			"modelId": "grok-composer-2.5-fast",
			"inputTokens": 14080,
			"outputTokens": 32,
			"cachedReadTokens": 7628,
			"reasoningTokens": 0
		}
	}`))
	if usage.InputTokens != 6452 || usage.CachedInputTokens != 7628 ||
		usage.OutputTokens != 32 || usage.TotalTokens != 14112 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawReadsThoughtTokens(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"usage": {"inputTokens": 10, "outputTokens": 2, "thoughtTokens": 7}
	}`))
	if usage.InputTokens != 10 || usage.OutputTokens != 2 || usage.ReasoningOutputTokens != 7 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawIgnoresTelemetryTotalOnly(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"sessionUpdate": "agent_message_chunk",
		"content": {"type": "text", "text": "ok"},
		"_meta": {
			"totalTokens": 78220,
			"eventId": "event-1",
			"updateType": "AgentMessageChunk"
		}
	}`))
	if !usage.IsZero() {
		t.Fatalf("usage = %#v, want zero", usage)
	}
}

func TestUsageFromRawReadsClaudeAgentPromptResult(t *testing.T) {
	// claude-agent-acp@0.44.0 session/prompt result is already disjoint
	// (inputTokens excludes cache reads/writes); components pass through.
	usage := usageFromRaw(json.RawMessage(`{
		"stopReason": "end_turn",
		"usage": {
			"inputTokens": 6090,
			"outputTokens": 15,
			"cachedReadTokens": 0,
			"cachedWriteTokens": 17424,
			"totalTokens": 23529
		}
	}`))
	if usage.InputTokens != 6090 || usage.CachedInputTokens != 0 || usage.CachedWriteTokens != 17424 ||
		usage.OutputTokens != 15 || usage.TotalTokens != 23529 {
		t.Fatalf("usage = %#v", usage)
	}

	usage = usageFromRaw(json.RawMessage(`{
		"usage": {
			"inputTokens": 10,
			"outputTokens": 119,
			"cachedReadTokens": 22000,
			"cachedWriteTokens": 103,
			"totalTokens": 22232
		}
	}`))
	if usage.InputTokens != 10 || usage.CachedInputTokens != 22000 || usage.CachedWriteTokens != 103 ||
		usage.OutputTokens != 119 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawNormalizesOnceAcrossFragments(t *testing.T) {
	// Cached count nested in a details fragment, no reported total: the
	// fragment must not invent a total of its own (which would defeat the
	// inclusive detection on the full message).
	usage := usageFromRaw(json.RawMessage(`{
		"usage": {
			"prompt_tokens": 128,
			"completion_tokens": 16,
			"prompt_tokens_details": {"cached_tokens": 96}
		}
	}`))
	if usage.InputTokens != 32 || usage.CachedInputTokens != 96 || usage.TotalTokens != 144 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawKeepsDisjointVocabularyWithoutTotals(t *testing.T) {
	// Anthropic-style keys without a total must not be mistaken for
	// inclusive counting just because reads fit inside input.
	usage := usageFromRaw(json.RawMessage(`{
		"usage": {"inputTokens": 100, "cachedReadTokens": 50, "outputTokens": 15}
	}`))
	if usage.InputTokens != 100 || usage.CachedInputTokens != 50 || usage.TotalTokens != 165 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawKeepsAnthropicCacheReadInputTokensDisjoint(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"usage": {"input_tokens": 100, "cache_read_input_tokens": 50, "output_tokens": 15}
	}`))
	if usage.InputTokens != 100 || usage.CachedInputTokens != 50 || usage.OutputTokens != 15 || usage.TotalTokens != 165 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawReadsCodexUsageUpdateMeta(t *testing.T) {
	// Patched codex-acp relays the last request's TokenUsage (OpenAI-style
	// inclusive counting) in usage_update _meta.
	usage := usageFromRaw(json.RawMessage(`{
		"sessionUpdate": "usage_update",
		"used": 11102,
		"size": 258400,
		"_meta": {
			"lastTokenUsage": {
				"input_tokens": 10930,
				"cached_input_tokens": 8704,
				"output_tokens": 172,
				"reasoning_output_tokens": 64,
				"total_tokens": 11102
			}
		}
	}`))
	if usage.ContextTokens != 11102 || usage.ContextWindowTokens != 258400 {
		t.Fatalf("context = %d / %d", usage.ContextTokens, usage.ContextWindowTokens)
	}
	if usage.InputTokens != 2226 || usage.CachedInputTokens != 8704 ||
		usage.OutputTokens != 172 || usage.ReasoningOutputTokens != 64 || usage.TotalTokens != 11102 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawUsesCodexLastTokenUsageNotCumulativeTotal(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"payload": {
			"info": {
				"total_token_usage": {
					"input_tokens": 50000,
					"cached_input_tokens": 45000,
					"output_tokens": 2000,
					"total_tokens": 52000
				},
				"last_token_usage": {
					"input_tokens": 1000,
					"cached_input_tokens": 400,
					"output_tokens": 10,
					"total_tokens": 1010
				}
			}
		}
	}`))
	if usage.InputTokens != 600 || usage.CachedInputTokens != 400 ||
		usage.OutputTokens != 10 || usage.TotalTokens != 1010 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestUsageFromRawIgnoresCodexCumulativeTotalOnly(t *testing.T) {
	usage := usageFromRaw(json.RawMessage(`{
		"_meta": {
			"totalTokenUsage": {
				"input_tokens": 50000,
				"cached_input_tokens": 45000,
				"output_tokens": 2000,
				"total_tokens": 52000
			}
		}
	}`))
	if !usage.IsZero() {
		t.Fatalf("usage = %#v, want zero", usage)
	}
}

func TestACPUsageDoesNotDoubleCountUsageAndLastTokenUsage(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "mixed-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {"inputTokens": 100, "outputTokens": 20, "totalTokens": 120},
		"_meta": {
			"lastTokenUsage": {
				"input_tokens": 100,
				"output_tokens": 20,
				"total_tokens": 120
			}
		}
	}`))

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 100 || loaded.Usage.OutputTokens != 20 || loaded.Usage.TotalTokens != 120 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
}

func TestACPUsageSumsCodexLastTokenUsageUpdates(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-last-token-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	manager.recordRawUsage(job, json.RawMessage(`{
		"sessionUpdate": "usage_update",
		"used": 1010,
		"size": 258400,
		"_meta": {
			"lastTokenUsage": {
				"input_tokens": 1000,
				"cached_input_tokens": 400,
				"output_tokens": 10,
				"total_tokens": 1010
			}
		}
	}`))
	manager.recordRawUsage(job, json.RawMessage(`{
		"sessionUpdate": "usage_update",
		"used": 2020,
		"size": 258400,
		"_meta": {
			"lastTokenUsage": {
				"input_tokens": 2000,
				"cached_input_tokens": 1500,
				"output_tokens": 20,
				"total_tokens": 2020
			}
		}
	}`))

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 1100 || loaded.Usage.CachedInputTokens != 1900 ||
		loaded.Usage.OutputTokens != 30 || loaded.Usage.TotalTokens != 3030 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
	if loaded.Usage.ContextTokens != 2020 || loaded.Usage.ContextWindowTokens != 258400 {
		t.Fatalf("context = %d / %d, want 2020 / 258400", loaded.Usage.ContextTokens, loaded.Usage.ContextWindowTokens)
	}
	events, err := store.UsageEventsSince(time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	var total storage.Usage
	for _, event := range events {
		total.InputTokens += event.Usage.InputTokens
		total.CachedInputTokens += event.Usage.CachedInputTokens
		total.OutputTokens += event.Usage.OutputTokens
		total.TotalTokens += event.Usage.TotalTokens
	}
	if total.InputTokens != loaded.Usage.InputTokens ||
		total.CachedInputTokens != loaded.Usage.CachedInputTokens ||
		total.OutputTokens != loaded.Usage.OutputTokens ||
		total.TotalTokens != loaded.Usage.TotalTokens {
		t.Fatalf("event usage = %#v, thread usage = %#v", total, loaded.Usage)
	}
	daily, err := usagecore.NewService(store).Daily(usagecore.DailyQuery{Days: 1, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	wantDaily := usagecore.UsageTotals{
		InputTokens:       loaded.Usage.InputTokens,
		CachedInputTokens: loaded.Usage.CachedInputTokens,
		OutputTokens:      loaded.Usage.OutputTokens,
	}
	if len(daily) != 1 || daily[0].Usage != wantDaily {
		t.Fatalf("daily usage = %#v, want %#v", daily, wantDaily)
	}
}

func TestACPUsageSkipsRepeatedCodexLastTokenUsageUpdate(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "codex-duplicate-token-count", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	raw := json.RawMessage(`{
		"sessionUpdate": "usage_update",
		"used": 1010,
		"size": 258400,
		"_meta": {
			"lastTokenUsage": {
				"input_tokens": 1000,
				"cached_input_tokens": 400,
				"output_tokens": 10,
				"total_tokens": 1010
			}
		}
	}`)
	manager.recordRawUsage(job, raw)
	manager.recordRawUsage(job, raw)

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 600 || loaded.Usage.CachedInputTokens != 400 ||
		loaded.Usage.OutputTokens != 10 || loaded.Usage.TotalTokens != 1010 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
	events, err := store.UsageEventsSince(time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestUsageFromRawReadsUsageUpdate(t *testing.T) {
	// usage_update session updates (claude-agent-acp, codex-acp) report the
	// live context fill and window size and nothing else.
	usage := usageFromRaw(json.RawMessage(`{
		"sessionUpdate": "usage_update",
		"used": 23529,
		"size": 1000000
	}`))
	if usage.ContextTokens != 23529 || usage.ContextWindowTokens != 1000000 {
		t.Fatalf("usage = %#v", usage)
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 {
		t.Fatalf("usage_update leaked counters: %#v", usage)
	}
	if usage.IsZero() {
		t.Fatal("usage_update payload considered empty")
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
	// The write count marks this shape as already disjoint; nothing shifts.
	if usage.InputTokens != 200 || usage.CachedInputTokens != 160 || usage.CachedWriteTokens != 20 ||
		usage.OutputTokens != 30 || usage.ReasoningOutputTokens != 8 {
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

	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 10,
			"cache_read_input_tokens": 60
		}
	}`))
	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {
			"prompt_tokens": 120,
			"completion_tokens": 15,
			"cache_read_input_tokens": 60,
			"total_tokens": 135
		}
	}`))

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 60 || loaded.Usage.CachedInputTokens != 60 ||
		loaded.Usage.OutputTokens != 15 || loaded.Usage.TotalTokens != 135 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
}

func TestACPUsagePersistsMonotonicTurnSnapshot(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp-monotonic-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {
			"prompt_tokens": 120,
			"completion_tokens": 20,
			"total_tokens": 140
		}
	}`))
	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 10,
			"total_tokens": 110
		}
	}`))
	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {
			"prompt_tokens": 130,
			"completion_tokens": 25
		}
	}`))

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 130 || loaded.Usage.OutputTokens != 25 || loaded.Usage.TotalTokens != 155 {
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

	manager.recordRawUsage(job, json.RawMessage(`{
		"tokens": {
			"input": 200,
			"output": 30,
			"reasoning": 8,
			"cache": {"read": 160}
		}
	}`))

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The tokens shape's explicit cache split is disjoint vocabulary, so
	// input stays whole and the missing total derives from the components.
	if loaded.Usage.InputTokens != 200 || loaded.Usage.CachedInputTokens != 160 ||
		loaded.Usage.OutputTokens != 30 || loaded.Usage.ReasoningOutputTokens != 8 ||
		loaded.Usage.TotalTokens != 390 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
}

func TestACPUsagePersistsContextFromUsageUpdates(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp-context", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	// Streamed usage_update notifications, then the prompt-result usage.
	manager.recordRawUsage(job, json.RawMessage(`{"sessionUpdate":"usage_update","used":23516,"size":1000000}`))
	manager.recordRawUsage(job, json.RawMessage(`{"sessionUpdate":"usage_update","used":23529,"size":1000000}`))
	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {"inputTokens": 6090, "outputTokens": 15, "cachedWriteTokens": 17424, "totalTokens": 23529}
	}`))

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.ContextTokens != 23529 || loaded.Usage.ContextWindowTokens != 1000000 {
		t.Fatalf("context = %d / %d, want 23529 / 1000000", loaded.Usage.ContextTokens, loaded.Usage.ContextWindowTokens)
	}
	if loaded.Usage.InputTokens != 6090 || loaded.Usage.CachedWriteTokens != 17424 ||
		loaded.Usage.OutputTokens != 15 || loaded.Usage.TotalTokens != 23529 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}

	// codex-acp style turn: usage_update only, no prompt-result usage. The
	// context snapshot still lands while counters stay put.
	job.startTurn(CompletionInline, false, false, false)
	manager.recordRawUsage(job, json.RawMessage(`{"sessionUpdate":"usage_update","used":11102,"size":258400}`))

	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.ContextTokens != 11102 || loaded.Usage.ContextWindowTokens != 258400 {
		t.Fatalf("context = %d / %d, want 11102 / 258400", loaded.Usage.ContextTokens, loaded.Usage.ContextWindowTokens)
	}
	if loaded.Usage.InputTokens != 6090 || loaded.Usage.TotalTokens != 23529 {
		t.Fatalf("counters drifted: %#v", loaded.Usage)
	}
}

// recordRawUsage must persist on arrival, with no separate end-of-turn flush, so
// the trailing usage_update claude/codex/grok send after the prompt response
// returns is never dropped. Each recordRawUsage below is followed immediately by a
// reload — the data must already be durable.
func TestACPUsagePersistsOnArrival(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp-arrival-usage", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &Job{ID: session.ID, Slug: session.Slug, ACPSession: "acp-session", State: StateRunning}
	job.startTurn(CompletionInline, false, false, false)

	manager.recordRawUsage(job, json.RawMessage(`{"sessionUpdate":"usage_update","used":42000,"size":200000}`))
	if loaded, _ := store.LoadSession(session.ID); loaded.Usage.ContextTokens != 42000 || loaded.Usage.ContextWindowTokens != 200000 {
		t.Fatalf("context not persisted on arrival: %d / %d", loaded.Usage.ContextTokens, loaded.Usage.ContextWindowTokens)
	}

	manager.recordRawUsage(job, json.RawMessage(`{
		"usage": {"inputTokens": 1000, "outputTokens": 50, "totalTokens": 1050}
	}`))
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Usage.InputTokens != 1000 || loaded.Usage.OutputTokens != 50 || loaded.Usage.TotalTokens != 1050 {
		t.Fatalf("counters not persisted on arrival: %#v", loaded.Usage)
	}
	if loaded.Usage.ContextTokens != 42000 || loaded.Usage.ContextWindowTokens != 200000 {
		t.Fatalf("context regressed after counter update: %d / %d", loaded.Usage.ContextTokens, loaded.Usage.ContextWindowTokens)
	}
}
