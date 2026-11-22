package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestCloudMirrorStoreLifecycleAndUnifiedUsage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	acc := storage.CloudAccount{
		BaseURL:          "https://jaz-cloud-staging.camadvanced.com",
		Token:            "secret-jwt-token-123",
		OIDCLoginURL:     "/v1/auth/login",
		UserID:           "usr_1",
		OrgID:            "org_1",
		OrgName:          "CAS",
		Email:            "user@example.com",
		DisplayName:      "Test User",
		Role:             "admin",
		CapabilitiesJSON: json.RawMessage(`{"spaces":true}`),
		ConnectedAt:      now,
		LastSyncAt:       now,
	}
	if err := store.SaveCloudAccount(ctx, acc); err != nil {
		t.Fatalf("SaveCloudAccount: %v", err)
	}

	// Token must be encrypted on disk and decrypted transparently on load.
	var rawStored string
	if err := store.db.QueryRowContext(ctx, `SELECT token_encrypted FROM cloud_account WHERE id = 1`).Scan(&rawStored); err != nil {
		t.Fatalf("select raw token: %v", err)
	}
	if rawStored == acc.Token || rawStored == "" {
		t.Fatalf("expected encrypted token on disk, got %q", rawStored)
	}
	loaded, ok, err := store.LoadCloudAccount(ctx)
	if err != nil || !ok {
		t.Fatalf("LoadCloudAccount: ok=%v err=%v", ok, err)
	}
	if loaded.Token != acc.Token || loaded.OrgID != "org_1" || loaded.OrgName != "CAS" {
		t.Fatalf("loaded account mismatch: %+v", loaded)
	}

	// Spaces & threads + incremental events + delete reconciliation.
	if err := store.ReplaceCloudSpaces(ctx, []storage.CloudSpace{
		{ID: "sp_1", OrgID: "org_1", Name: "Engineering", CreatedBy: "usr_1", CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("ReplaceCloudSpaces: %v", err)
	}
	if err := store.SyncCloudThreads(ctx, []storage.CloudThread{
		{ID: "ct_1", OrgID: "org_1", OwnerUserID: "usr_1", SpaceID: "sp_1", RunID: "ct_1", Visibility: "space", Title: "Alpha", Status: "idle", ModelProfileID: "vertex", Model: "google/gemini-4-neon", CreatedAt: now, UpdatedAt: now},
		{ID: "ct_2", OrgID: "org_1", OwnerUserID: "usr_1", RunID: "ct_2", Visibility: "private", Title: "Beta", Status: "idle", CreatedAt: now, UpdatedAt: now.Add(-time.Minute)},
	}); err != nil {
		t.Fatalf("SyncCloudThreads: %v", err)
	}
	if err := store.AppendCloudThreadEvents(ctx, "ct_1", []storage.CloudThreadEvent{
		{ThreadID: "ct_1", Seq: 1, OrgID: "org_1", Type: "user_message", PayloadJSON: json.RawMessage(`{"text":"hi"}`), CreatedAt: now},
		{ThreadID: "ct_1", Seq: 2, OrgID: "org_1", Type: "assistant_message", PayloadJSON: json.RawMessage(`{"text":"hello"}`), CreatedAt: now.Add(time.Second)},
	}); err != nil {
		t.Fatalf("AppendCloudThreadEvents: %v", err)
	}
	// Syncing again without ct_2 removes ct_2 but preserves ct_1's last_seq and events.
	if err := store.SyncCloudThreads(ctx, []storage.CloudThread{
		{ID: "ct_1", OrgID: "org_1", OwnerUserID: "usr_1", SpaceID: "sp_1", RunID: "ct_1", Visibility: "space", Title: "Alpha Updated", Status: "idle", ModelProfileID: "vertex", Model: "google/gemini-4-neon", CreatedAt: now, UpdatedAt: now.Add(time.Minute)},
	}); err != nil {
		t.Fatalf("SyncCloudThreads 2: %v", err)
	}
	ct1, ok, err := store.GetCloudThread(ctx, "ct_1")
	if err != nil || !ok {
		t.Fatalf("GetCloudThread ct_1: ok=%v err=%v", ok, err)
	}
	if ct1.Title != "Alpha Updated" || ct1.LastSeq != 2 {
		t.Fatalf("ct_1 mismatch after resync: title=%q last_seq=%d", ct1.Title, ct1.LastSeq)
	}
	if _, ok, err := store.GetCloudThread(ctx, "ct_2"); err != nil || ok {
		t.Fatalf("expected ct_2 deleted on resync, ok=%v err=%v", ok, err)
	}
	evs, err := store.ListCloudThreadEvents(ctx, "ct_1", 1)
	if err != nil || len(evs) != 1 || evs[0].Seq != 2 {
		t.Fatalf("ListCloudThreadEvents(after=1): %+v err=%v", evs, err)
	}

	// Instructions mirror (including long_term and short_term).
	if err := store.UpsertCloudInstruction(ctx, storage.CloudInstruction{
		OrgID: "org_1", Scope: "user", ScopeID: "usr_1", Kind: "short_term",
		BodyMD: "active sprint", Version: "v1", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertCloudInstruction: %v", err)
	}
	inst, ok, err := store.GetCloudInstruction(ctx, "org_1", "user", "usr_1", "short_term")
	if err != nil || !ok || inst.BodyMD != "active sprint" {
		t.Fatalf("GetCloudInstruction: %+v ok=%v err=%v", inst, ok, err)
	}

	// Unified UsageEvents must include both local usage_events and mirrored cloud_usage_events.
	localSess := storage.Session{ID: "local_1", Slug: "local_1", Runtime: storage.RuntimeACP, Status: storage.StatusIdle, ModelProvider: "openai", Model: "gpt-6.1-sol", CreatedAt: now, UpdatedAt: now}
	if err := store.SaveSession(localSess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.AddUsage("local_1", storage.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}); err != nil {
		t.Fatalf("AddUsage: %v", err)
	}
	if err := store.UpsertCloudUsageEvents(ctx, []storage.CloudUsageEvent{
		{ID: 42, OrgID: "org_1", UserID: "usr_1", ThreadID: "ct_1", ModelProfileID: "vertex", Model: "google/gemini-4-neon", InputTokens: 300, CachedInputTokens: 100, OutputTokens: 80, CostUSD: 0.01, CreatedAt: now.Add(time.Second)},
		{ID: 42, OrgID: "org_1", UserID: "usr_1", ThreadID: "ct_1", ModelProfileID: "vertex", Model: "google/gemini-4-neon", InputTokens: 300, CachedInputTokens: 100, OutputTokens: 80, CostUSD: 0.01, CreatedAt: now.Add(time.Second)},
	}); err != nil {
		t.Fatalf("UpsertCloudUsageEvents: %v", err)
	}
	maxID, err := store.MaxCloudUsageEventID(ctx)
	if err != nil || maxID != 42 {
		t.Fatalf("MaxCloudUsageEventID = %d err=%v", maxID, err)
	}
	combined, err := store.UsageEvents(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("UsageEvents: %v", err)
	}
	if len(combined) != 2 {
		t.Fatalf("expected 2 combined usage events (1 local + 1 deduped cloud), got %d: %+v", len(combined), combined)
	}

	// Key-loss guard refuses to start if secrets.key is missing while cloud_account holds encrypted token.
	_ = store.Close()
	keyPath := filepath.Join(root, "secrets.key")
	savedKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read secrets.key: %v", err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatalf("remove secrets.key: %v", err)
	}
	if _, err := New(root); err == nil {
		t.Fatalf("expected New() to fail when secrets.key is missing with encrypted cloud_account token")
	}
	if err := os.WriteFile(keyPath, savedKey, 0o600); err != nil {
		t.Fatalf("restore secrets.key: %v", err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatalf("reopen New: %v", err)
	}
	defer store.Close()

	// ClearCloudMirror wipes cloud tables while preserving local threads & usage.
	if err := store.ClearCloudMirror(ctx); err != nil {
		t.Fatalf("ClearCloudMirror: %v", err)
	}
	if _, ok, err := store.LoadCloudAccount(ctx); err != nil || ok {
		t.Fatalf("expected cloud account cleared, ok=%v err=%v", ok, err)
	}
	remainingUsage, err := store.UsageEvents(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil || len(remainingUsage) != 1 || remainingUsage[0].SessionID != "local_1" {
		t.Fatalf("expected only local usage to remain after ClearCloudMirror, got %+v err=%v", remainingUsage, err)
	}
}
