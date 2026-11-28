package sqlite

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestCloudMirrorMultiWorkspaceIsolationAndUsage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	accA := storage.CloudAccount{
		ID:               "org_a:usr_1@https://a.example.com",
		BaseURL:          "https://a.example.com",
		Token:            "secret-jwt-a",
		RefreshToken:     "rt_secret_a",
		OIDCLoginURL:     "/v1/auth/login",
		UserID:           "usr_1",
		OrgID:            "org_a",
		OrgName:          "Alpha Org",
		Email:            "a@example.com",
		DisplayName:      "Alice",
		Role:             "admin",
		CapabilitiesJSON: json.RawMessage(`{"spaces":true}`),
		ConnectedAt:      now,
		LastSyncAt:       now,
	}
	accB := storage.CloudAccount{
		ID:               "org_b:usr_2@https://b.example.com",
		BaseURL:          "https://b.example.com",
		Token:            "secret-jwt-b",
		OIDCLoginURL:     "/v1/auth/login",
		UserID:           "usr_2",
		OrgID:            "org_b",
		OrgName:          "Beta Org",
		Email:            "b@example.com",
		DisplayName:      "Bob",
		Role:             "member",
		CapabilitiesJSON: json.RawMessage(`{"spaces":true}`),
		ConnectedAt:      now.Add(time.Second),
		LastSyncAt:       now.Add(time.Second),
	}
	for _, acc := range []storage.CloudAccount{accA, accB} {
		if err := store.SaveCloudAccount(ctx, acc); err != nil {
			t.Fatalf("SaveCloudAccount %s: %v", acc.ID, err)
		}
	}

	// Tokens must be encrypted on disk and decrypted transparently on load.
	var rawStored string
	if err := store.db.QueryRowContext(ctx, `SELECT token_encrypted FROM cloud_accounts WHERE id = ?`, accA.ID).Scan(&rawStored); err != nil {
		t.Fatalf("select raw token: %v", err)
	}
	if rawStored == accA.Token || rawStored == "" {
		t.Fatalf("expected encrypted token on disk, got %q", rawStored)
	}
	accounts, err := store.ListCloudAccounts(ctx)
	if err != nil || len(accounts) != 2 || accounts[0].ID != accA.ID || accounts[1].ID != accB.ID {
		t.Fatalf("ListCloudAccounts = %+v err=%v", accounts, err)
	}
	loadedA, ok, err := store.LoadCloudAccount(ctx, accA.ID)
	if err != nil || !ok || loadedA.Token != accA.Token || loadedA.RefreshToken != accA.RefreshToken || loadedA.OrgName != "Alpha Org" {
		t.Fatalf("LoadCloudAccount A: %+v ok=%v err=%v", loadedA, ok, err)
	}

	// Default active workspace is "local"; Save/Load persists cleanly.
	if ws, err := store.LoadActiveWorkspaceID(ctx); err != nil || ws != storage.LocalWorkspaceID {
		t.Fatalf("default active workspace = %q err=%v", ws, err)
	}
	if err := store.ApplyCloudWorkspaceChange(ctx, storage.CloudWorkspaceChange{ActiveID: accA.ID}); err != nil {
		t.Fatalf("activate account A: %v", err)
	}
	if ws, err := store.LoadActiveWorkspaceID(ctx); err != nil || ws != accA.ID {
		t.Fatalf("loaded active workspace = %q err=%v", ws, err)
	}

	// Spaces, threads (including identical thread IDs across different accounts!), events, instructions stay isolated.
	if err := store.ReplaceCloudSpaces(ctx, accA.ID, []storage.CloudSpace{
		{ID: "sp_shared", OrgID: "org_a", Name: "Alpha Eng", CreatedBy: "usr_1", CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("ReplaceCloudSpaces A: %v", err)
	}
	if err := store.ReplaceCloudSpaces(ctx, accB.ID, []storage.CloudSpace{
		{ID: "sp_shared", OrgID: "org_b", Name: "Beta Ops", CreatedBy: "usr_2", CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("ReplaceCloudSpaces B: %v", err)
	}
	spacesA, err := store.ListCloudSpaces(ctx, accA.ID)
	if err != nil || len(spacesA) != 1 || spacesA[0].Name != "Alpha Eng" {
		t.Fatalf("ListCloudSpaces A: %+v err=%v", spacesA, err)
	}
	spacesB, err := store.ListCloudSpaces(ctx, accB.ID)
	if err != nil || len(spacesB) != 1 || spacesB[0].Name != "Beta Ops" {
		t.Fatalf("ListCloudSpaces B: %+v err=%v", spacesB, err)
	}

	if err := store.SyncCloudThreads(ctx, accA.ID, []storage.CloudThread{
		{ID: "ct_1", OrgID: "org_a", OwnerUserID: "usr_1", SpaceID: "sp_shared", RunID: "ct_1", Visibility: "space", Title: "Alpha 1", Status: "idle", ModelProfileID: "vertex", Model: "google/gemini-4-neon", CreatedAt: now, UpdatedAt: now},
		{ID: "ct_2", OrgID: "org_a", OwnerUserID: "usr_1", RunID: "ct_2", Visibility: "private", Title: "Alpha 2", Status: "idle", CreatedAt: now, UpdatedAt: now.Add(-time.Minute)},
	}); err != nil {
		t.Fatalf("SyncCloudThreads A: %v", err)
	}
	if err := store.SyncCloudThreads(ctx, accB.ID, []storage.CloudThread{
		{ID: "ct_1", OrgID: "org_b", OwnerUserID: "usr_2", SpaceID: "sp_shared", RunID: "ct_1", Visibility: "space", Title: "Beta 1", Status: "idle", ModelProfileID: "anthropic", Model: "claude-4", CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("SyncCloudThreads B: %v", err)
	}
	if err := store.AppendCloudThreadEvents(ctx, accA.ID, "ct_1", []storage.CloudThreadEvent{
		{ThreadID: "ct_1", Seq: 1, OrgID: "org_a", Type: "user_message", PayloadJSON: json.RawMessage(`{"text":"hi alpha"}`), CreatedAt: now},
		{ThreadID: "ct_1", Seq: 2, OrgID: "org_a", Type: "assistant_message", PayloadJSON: json.RawMessage(`{"text":"hello alpha"}`), CreatedAt: now.Add(time.Second)},
	}); err != nil {
		t.Fatalf("AppendCloudThreadEvents A: %v", err)
	}
	if err := store.AppendCloudThreadEvents(ctx, accB.ID, "ct_1", []storage.CloudThreadEvent{
		{ThreadID: "ct_1", Seq: 1, OrgID: "org_b", Type: "user_message", PayloadJSON: json.RawMessage(`{"text":"hi beta"}`), CreatedAt: now},
	}); err != nil {
		t.Fatalf("AppendCloudThreadEvents B: %v", err)
	}

	// Resyncing A without ct_2 removes ct_2 from A, preserves A's ct_1 last_seq, and never touches B.
	if err := store.SyncCloudThreads(ctx, accA.ID, []storage.CloudThread{
		{ID: "ct_1", OrgID: "org_a", OwnerUserID: "usr_1", SpaceID: "sp_shared", RunID: "ct_1", Visibility: "space", Title: "Alpha 1 Updated", Status: "idle", ModelProfileID: "vertex", Model: "google/gemini-4-neon", CreatedAt: now, UpdatedAt: now.Add(time.Minute)},
	}); err != nil {
		t.Fatalf("SyncCloudThreads A 2: %v", err)
	}
	ct1A, ok, err := store.GetCloudThread(ctx, accA.ID, "ct_1")
	if err != nil || !ok || ct1A.Title != "Alpha 1 Updated" || ct1A.LastSeq != 2 {
		t.Fatalf("GetCloudThread A ct_1: %+v ok=%v err=%v", ct1A, ok, err)
	}
	if _, ok, err := store.GetCloudThread(ctx, accA.ID, "ct_2"); err != nil || ok {
		t.Fatalf("expected ct_2 deleted from A, ok=%v err=%v", ok, err)
	}
	ct1B, ok, err := store.GetCloudThread(ctx, accB.ID, "ct_1")
	if err != nil || !ok || ct1B.Title != "Beta 1" || ct1B.LastSeq != 1 {
		t.Fatalf("GetCloudThread B ct_1: %+v ok=%v err=%v", ct1B, ok, err)
	}
	evsA, err := store.ListCloudThreadEvents(ctx, accA.ID, "ct_1", 1)
	if err != nil || len(evsA) != 1 || evsA[0].Seq != 2 {
		t.Fatalf("ListCloudThreadEvents A(after=1): %+v err=%v", evsA, err)
	}

	// Instructions scoped by account_id.
	if err := store.UpsertCloudInstruction(ctx, accA.ID, storage.CloudInstruction{
		OrgID: "org_a", Scope: "org", ScopeID: "org_a", Kind: "agents",
		BodyMD: "Alpha org rules", Version: "v1", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertCloudInstruction A: %v", err)
	}
	if err := store.UpsertCloudInstruction(ctx, accB.ID, storage.CloudInstruction{
		OrgID: "org_b", Scope: "org", ScopeID: "org_b", Kind: "agents",
		BodyMD: "Beta org rules", Version: "v1", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertCloudInstruction B: %v", err)
	}
	if _, ok, err := store.GetCloudInstruction(ctx, accA.ID, "org_b", "org", "org_b", "agents"); err != nil || ok {
		t.Fatalf("expected A not to see B's instructions: ok=%v err=%v", ok, err)
	}

	// Strict workspace-scoped UsageEvents:
	// - local session in "local"
	// - local ACP session created inside Cloud A
	// - cloud_usage_events for Cloud A and Cloud B
	localSess, err := store.CreateSession(storage.CreateSession{
		Slug: "local-only", Runtime: storage.RuntimeACP, ModelProvider: "openai", Model: "gpt-local",
	})
	if err != nil {
		t.Fatalf("CreateSession local: %v", err)
	}
	cloudALocalSess, err := store.CreateSession(storage.CreateSession{
		Slug: "cloud-a-mac-acp", Runtime: storage.RuntimeACP, WorkspaceID: accA.ID, ModelProvider: "openai", Model: "gpt-cloud-a-local",
	})
	if err != nil {
		t.Fatalf("CreateSession cloudA local: %v", err)
	}
	if err := store.AddUsage(localSess.ID, storage.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}); err != nil {
		t.Fatalf("AddUsage local: %v", err)
	}
	if err := store.AddUsage(cloudALocalSess.ID, storage.Usage{InputTokens: 200, OutputTokens: 60, TotalTokens: 260}); err != nil {
		t.Fatalf("AddUsage cloudA local: %v", err)
	}
	if err := store.UpsertCloudUsageEvents(ctx, accA.ID, []storage.CloudUsageEvent{
		{ID: 42, OrgID: "org_a", UserID: "usr_1", ThreadID: "ct_1", ModelProfileID: "vertex", Model: "gemini-a", InputTokens: 300, CachedInputTokens: 100, OutputTokens: 80, CostUSD: 0.01, CreatedAt: now.Add(time.Second)},
		{ID: 42, OrgID: "org_a", UserID: "usr_1", ThreadID: "ct_1", ModelProfileID: "vertex", Model: "gemini-a", InputTokens: 300, CachedInputTokens: 100, OutputTokens: 80, CostUSD: 0.01, CreatedAt: now.Add(time.Second)},
	}); err != nil {
		t.Fatalf("UpsertCloudUsageEvents A: %v", err)
	}
	if err := store.UpsertCloudUsageEvents(ctx, accB.ID, []storage.CloudUsageEvent{
		{ID: 42, OrgID: "org_b", UserID: "usr_2", ThreadID: "ct_1", ModelProfileID: "anthropic", Model: "claude-b", InputTokens: 500, OutputTokens: 120, CostUSD: 0.02, CreatedAt: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("UpsertCloudUsageEvents B: %v", err)
	}

	windowStart, windowEnd := now.Add(-time.Minute), now.Add(time.Minute)
	localUsage, err := store.UsageEventsForWorkspace(storage.LocalWorkspaceID, windowStart, windowEnd)
	if err != nil || len(localUsage) != 1 || localUsage[0].SessionID != localSess.ID {
		t.Fatalf("local usage must contain ONLY local session: %+v err=%v", localUsage, err)
	}
	cloudAUsage, err := store.UsageEventsForWorkspace(accA.ID, windowStart, windowEnd)
	if err != nil || len(cloudAUsage) != 2 {
		t.Fatalf("cloudA usage must contain cloudA local ACP + cloudA cloud event (2 total): %+v err=%v", cloudAUsage, err)
	}
	cloudBUsage, err := store.UsageEventsForWorkspace(accB.ID, windowStart, windowEnd)
	if err != nil || len(cloudBUsage) != 1 || cloudBUsage[0].Model != "claude-b" {
		t.Fatalf("cloudB usage must contain ONLY cloudB event: %+v err=%v", cloudBUsage, err)
	}

	// Store.UsageEvents follows persisted active_workspace_id (currently accA.ID).
	activeUsage, err := store.UsageEvents(windowStart, windowEnd)
	if err != nil || len(activeUsage) != 2 {
		t.Fatalf("UsageEvents with active=accA: %+v err=%v", activeUsage, err)
	}

	// Key-loss guard refuses to start if secrets.key is missing while cloud_accounts holds encrypted tokens.
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
		t.Fatalf("expected New() to fail when secrets.key is missing with encrypted cloud_accounts tokens")
	}
	if err := os.WriteFile(keyPath, savedKey, 0o600); err != nil {
		t.Fatalf("restore secrets.key: %v", err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatalf("reopen New: %v", err)
	}
	defer store.Close()

	// Removing accA wipes its mirrored tables, preserving accB and local.
	if err := store.ApplyCloudWorkspaceChange(ctx, storage.CloudWorkspaceChange{ActiveID: storage.LocalWorkspaceID, RemoveIDs: []string{accA.ID}}); err != nil {
		t.Fatalf("DeleteCloudAccount A: %v", err)
	}
	if _, ok, err := store.LoadCloudAccount(ctx, accA.ID); err != nil || ok {
		t.Fatalf("expected accA deleted, ok=%v err=%v", ok, err)
	}
	if loadedB, ok, err := store.LoadCloudAccount(ctx, accB.ID); err != nil || !ok || loadedB.Token != accB.Token {
		t.Fatalf("expected accB preserved after deleting accA: %+v ok=%v err=%v", loadedB, ok, err)
	}
	remainingBThreads, err := store.ListCloudThreads(ctx, accB.ID, false)
	if err != nil || len(remainingBThreads) != 1 || remainingBThreads[0].Title != "Beta 1" {
		t.Fatalf("expected accB threads preserved: %+v err=%v", remainingBThreads, err)
	}
	remainingBUsage, err := store.UsageEventsForWorkspace(accB.ID, windowStart, windowEnd)
	if err != nil || len(remainingBUsage) != 1 || remainingBUsage[0].Model != "claude-b" {
		t.Fatalf("expected accB usage preserved: %+v err=%v", remainingBUsage, err)
	}
	remainingLocalUsage, err := store.UsageEventsForWorkspace(storage.LocalWorkspaceID, windowStart, windowEnd)
	if err != nil || len(remainingLocalUsage) != 1 || remainingLocalUsage[0].SessionID != localSess.ID {
		t.Fatalf("expected local usage preserved: %+v err=%v", remainingLocalUsage, err)
	}
}

func TestWorkspacesMigrationUpgradesLegacySingleCloudAccount(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	localSess, err := store.CreateSession(storage.CreateSession{Slug: "pre-upgrade-local", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(localSess.ID, storage.Usage{InputTokens: 75, OutputTokens: 25, TotalTokens: 100}); err != nil {
		t.Fatal(err)
	}
	encryptedTok, err := store.encryptSecret(cloudAccountSecretID, []byte("legacy-jwt"))
	if err != nil {
		t.Fatal(err)
	}

	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 94); err != nil {
		t.Fatal(err)
	}

	nowMs := time.Now().UTC().UnixMilli()
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO cloud_account (id, base_url, token_encrypted, user_id, org_id, org_name, email, connected_at_ms) VALUES (1, ?, ?, 'usr_leg', 'org_leg', 'Legacy Org', 'leg@example.com', ?)`,
			[]any{"https://legacy.example.com/", encryptedTok, nowMs},
		},
		{
			`INSERT INTO cloud_spaces (id, org_id, name, created_at_ms, updated_at_ms) VALUES ('sp_leg', 'org_leg', 'Legacy Space', ?, ?)`,
			[]any{nowMs, nowMs},
		},
		{
			`INSERT INTO cloud_threads (id, org_id, owner_user_id, run_id, title, created_at_ms, updated_at_ms) VALUES ('ct_leg', 'org_leg', 'usr_leg', 'ct_leg', 'Legacy Cloud Thread', ?, ?)`,
			[]any{nowMs, nowMs},
		},
		{
			`INSERT INTO cloud_thread_events (thread_id, seq, org_id, type, payload_json, created_at_ms) VALUES ('ct_leg', 1, 'org_leg', 'user_message', '{"text":"hello"}', ?)`,
			[]any{nowMs},
		},
		{
			`INSERT INTO cloud_instructions (org_id, scope, scope_id, kind, body_md, version, updated_at_ms) VALUES ('org_leg', 'org', 'org_leg', 'agents', 'Legacy Org Rule', 'v1', ?)`,
			[]any{nowMs},
		},
		{
			`INSERT INTO cloud_usage_events (id, org_id, user_id, thread_id, model_profile_id, model, input_tokens, output_tokens, created_at_ms) VALUES (9, 'org_leg', 'usr_leg', 'ct_leg', 'vertex', 'gemini-leg', 400, 100, ?)`,
			[]any{nowMs},
		},
	} {
		if _, err := store.db.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatalf("seed legacy table: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening runs 0095_workspaces.sql and protectSecrets against migrated cloud_accounts.
	upgraded, err := New(root)
	if err != nil {
		t.Fatalf("reopen after 0095 upgrade: %v", err)
	}
	defer upgraded.Close()

	wantAccountID := "org_leg:usr_leg@https://legacy.example.com"
	activeWS, err := upgraded.LoadActiveWorkspaceID(ctx)
	if err != nil || activeWS != wantAccountID {
		t.Fatalf("active workspace after upgrade = %q, want %q (err=%v)", activeWS, wantAccountID, err)
	}
	acc, ok, err := upgraded.LoadCloudAccount(ctx, wantAccountID)
	if err != nil || !ok || acc.Token != "legacy-jwt" || acc.OrgName != "Legacy Org" {
		t.Fatalf("migrated account = %+v ok=%v err=%v", acc, ok, err)
	}
	spaces, err := upgraded.ListCloudSpaces(ctx, wantAccountID)
	if err != nil || len(spaces) != 1 || spaces[0].Name != "Legacy Space" {
		t.Fatalf("migrated spaces = %+v err=%v", spaces, err)
	}
	threads, err := upgraded.ListCloudThreads(ctx, wantAccountID, false)
	if err != nil || len(threads) != 1 || threads[0].Title != "Legacy Cloud Thread" {
		t.Fatalf("migrated threads = %+v err=%v", threads, err)
	}
	evs, err := upgraded.ListCloudThreadEvents(ctx, wantAccountID, "ct_leg", 0)
	if err != nil || len(evs) != 1 || evs[0].Seq != 1 {
		t.Fatalf("migrated events = %+v err=%v", evs, err)
	}
	inst, ok, err := upgraded.GetCloudInstruction(ctx, wantAccountID, "org_leg", "org", "org_leg", "agents")
	if err != nil || !ok || inst.BodyMD != "Legacy Org Rule" {
		t.Fatalf("migrated instruction = %+v ok=%v err=%v", inst, ok, err)
	}
	cloudUsage, err := upgraded.UsageEventsForWorkspace(wantAccountID, time.UnixMilli(nowMs-60_000), time.UnixMilli(nowMs+60_000))
	if err != nil || len(cloudUsage) != 1 || cloudUsage[0].Model != "gemini-leg" {
		t.Fatalf("migrated cloud usage = %+v err=%v", cloudUsage, err)
	}
	localSessAfter, err := upgraded.LoadSession(localSess.ID)
	if err != nil || localSessAfter.WorkspaceID != storage.LocalWorkspaceID {
		t.Fatalf("existing local session must stay in 'local', got %+v err=%v", localSessAfter, err)
	}
	localUsage, err := upgraded.UsageEventsForWorkspace(storage.LocalWorkspaceID, time.UnixMilli(nowMs-60_000), time.UnixMilli(nowMs+60_000))
	if err != nil || len(localUsage) != 1 || localUsage[0].SessionID != localSess.ID {
		t.Fatalf("existing local usage must stay in 'local', got %+v err=%v", localUsage, err)
	}
}
