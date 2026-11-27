package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/machinebridge"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestMachineBridgeSupervisorFollowsActiveCloudWorkspace(t *testing.T) {
	ctx := t.Context()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	var (
		mu             sync.Mutex
		connectedHello []machinebridge.HelloRequest
		connectedAuths []string
	)
	connectedCh := make(chan struct{}, 4)
	closedCh := make(chan struct{}, 4)
	pending := make(chan struct{}, 1)

	newMockCloud := func(orgID, orgName, token string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/jaz-server":
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
			case "/v1/auth/me":
				_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": orgID, "org_name": orgName, "email": "a@cas.dev", "display_name": "Alice", "role": "admin"})
			case "/v1/spaces":
				_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
			case "/v1/threads":
				_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
			case "/v1/usage/events":
				_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
			case "/v1/models":
				_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
			case "/v1/machines":
				_ = json.NewEncoder(w).Encode(map[string]any{"machines": []any{}})
			case "/v1/usage/summary":
				_ = json.NewEncoder(w).Encode(map[string]any{})
			case "/v1/auth/logout":
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			case "/v1/machines/connect":
				authHeader := r.Header.Get("Authorization")
				if authHeader != "Bearer "+token {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer func() {
					_ = conn.Close()
					select {
					case closedCh <- struct{}{}:
					default:
					}
				}()
				var frame machinebridge.Frame
				if err := conn.ReadJSON(&frame); err != nil || frame.Method != machinebridge.MethodHello {
					return
				}
				var hello machinebridge.HelloRequest
				if err := json.Unmarshal(frame.Params, &hello); err != nil {
					return
				}
				mu.Lock()
				connectedAuths = append(connectedAuths, authHeader)
				connectedHello = append(connectedHello, hello)
				mu.Unlock()
				respBytes, _ := json.Marshal(machinebridge.HelloResponse{
					ProtocolVersion: machinebridge.CurrentProtocolVersion,
					Accepted:        true,
					Capabilities:    machinebridge.Capabilities{ACPProfiles: true, FileStaging: true, MaxStageBytes: 100 << 20},
				})
				_ = conn.WriteJSON(machinebridge.Frame{ID: frame.ID, Result: respBytes})
				select {
				case connectedCh <- struct{}{}:
				default:
				}
				for {
					var request machinebridge.Frame
					if conn.ReadJSON(&request) != nil {
						return
					}
					var params struct {
						Profile  string `json:"profile_id"`
						Rejected int64  `json:"rejected_version"`
					}
					if request.Method != "acp.profile.resolve" || json.Unmarshal(request.Params, &params) != nil {
						t.Error("unexpected machine request")
						return
					}
					switch params.Profile {
					case "acp:codex":
						if params.Rejected != 0 && params.Rejected != 11 {
							t.Error("rejected credential version was lost")
						}
						if params.Rejected == 11 {
							pending <- struct{}{}
							continue
						}
						result := json.RawMessage(`{"profile_id":"acp:codex","agent":"codex","credential_kind":"oauth_bundle","version":11,"chatgpt":{"accessToken":"fake-access","chatgptAccountId":"test"}}`)
						_ = conn.WriteJSON(machinebridge.Frame{ID: request.ID, Result: result})
					default:
						_ = conn.WriteJSON(machinebridge.Frame{ID: request.ID, Error: &machinebridge.RPCError{Code: machinebridge.CodeNotFound}})
					}
				}
			default:
				if strings.HasPrefix(r.URL.Path, "/v1/instructions/") {
					_ = json.NewEncoder(w).Encode(map[string]any{"body_md": "", "version": ""})
					return
				}
				http.NotFound(w, r)
			}
		}))
	}

	cloudA := newMockCloud("org-a", "Acme", "tok-a")
	defer cloudA.Close()
	cloudB := newMockCloud("org-b", "Beta", "tok-b")
	defer cloudB.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	workspace := t.TempDir()
	cloud := cloudsync.New(store, nil)
	bridge := NewMachineBridgeSupervisor(cloud, store, workspace, nil)
	bridge.minBackoff = 20 * time.Millisecond
	bridge.maxBackoff = 100 * time.Millisecond
	cloud.AddOnAccountChanged(bridge.Reconcile)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	bridge.Start(runCtx)
	defer bridge.Stop(context.Background())

	// 1. Connecting Cloud A auto-connects machinebridge to Cloud A.
	stA, err := cloud.Connect(ctx, cloudsync.ConnectRequest{URL: cloudA.URL, Token: "tok-a"})
	if err != nil {
		t.Fatalf("Connect A: %v", err)
	}
	select {
	case <-connectedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for machinebridge connect to Cloud A")
	}
	mu.Lock()
	if len(connectedHello) != 1 || connectedAuths[0] != "Bearer tok-a" || connectedHello[0].DeviceID == "" || !connectedHello[0].Capabilities.FileStaging || len(connectedHello[0].FolderGrants) != 1 {
		t.Fatalf("unexpected hello A: auths=%v hellos=%+v", connectedAuths, connectedHello)
	}
	deviceID := connectedHello[0].DeviceID
	mu.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for !bridge.Status().Connected && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	profile, err := bridge.ResolveACPProfile(ctx, stA.ActiveWorkspaceID, "acp:codex", 0)
	if err != nil || profile.ChatGPT == nil || profile.ChatGPT.AccountID != "test" || profile.Version != 11 {
		t.Fatalf("could not resolve the Codex profile over the paired socket: %v", err)
	}
	resolveErr := make(chan error, 1)
	go func() {
		_, err := bridge.ResolveACPProfile(ctx, stA.ActiveWorkspaceID, "acp:codex", 11)
		resolveErr <- err
	}()
	select {
	case <-pending:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an in-flight credential request")
	}

	// 2. Switching to local cleanly disconnects machinebridge without removing Cloud A.
	if _, err := cloud.SwitchWorkspace(ctx, cloudsync.LocalWorkspaceID); err != nil {
		t.Fatalf("SwitchWorkspace local: %v", err)
	}
	select {
	case <-closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for machinebridge disconnect on switch to local")
	}

	select {
	case err := <-resolveErr:
		if !errors.Is(err, cloudsync.ErrWorkspaceChanged) {
			t.Fatalf("pending credentials survived workspace switch: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending credential request did not fail on disconnect")
	}

	// 3. Connecting Cloud B connects machinebridge to Cloud B with the same persisted device_id.
	if _, err := cloud.Connect(ctx, cloudsync.ConnectRequest{URL: cloudB.URL, Token: "tok-b"}); err != nil {
		t.Fatalf("Connect B: %v", err)
	}
	select {
	case <-connectedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for machinebridge connect to Cloud B")
	}
	mu.Lock()
	if len(connectedHello) != 2 || connectedHello[1].DeviceID != deviceID || connectedAuths[1] != "Bearer tok-b" {
		t.Fatalf("expected stable device_id %q on Cloud B, got auths=%v hellos=%+v", deviceID, connectedAuths, connectedHello)
	}
	mu.Unlock()

	// 4. Switching from Cloud B back to Cloud A disconnects B and reconnects A.
	if _, err := cloud.SwitchWorkspace(ctx, stA.ActiveWorkspaceID); err != nil {
		t.Fatalf("SwitchWorkspace A: %v", err)
	}
	select {
	case <-closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Cloud B disconnect on switch to A")
	}
	select {
	case <-connectedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Cloud A reconnect")
	}
	mu.Lock()
	if len(connectedHello) != 3 || connectedHello[2].DeviceID != deviceID || connectedAuths[2] != "Bearer tok-a" {
		t.Fatalf("expected Cloud A reconnect, got auths=%v hellos=%+v", connectedAuths, connectedHello)
	}
	mu.Unlock()

	// 5. Disconnecting active Cloud A closes machinebridge cleanly.
	if err := cloud.Disconnect(ctx, ""); err != nil {
		t.Fatalf("Disconnect A: %v", err)
	}
	select {
	case <-closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for machinebridge disconnect on Disconnect")
	}
}

func TestMachineBridgeSupervisorLiveConfigUpdatesAndPersistence(t *testing.T) {
	ctx := t.Context()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	var (
		mu             sync.Mutex
		connectedHello []machinebridge.HelloRequest
	)
	connectedCh := make(chan struct{}, 8)
	closedCh := make(chan struct{}, 8)

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "org-a", "org_name": "Acme", "email": "a@cas.dev", "display_name": "Alice", "role": "admin"})
		case "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
		case "/v1/machines":
			_ = json.NewEncoder(w).Encode(map[string]any{"machines": []any{}})
		case "/v1/usage/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case "/v1/machines/connect":
			if r.Header.Get("Authorization") != "Bearer tok-live" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer func() {
				_ = conn.Close()
				select {
				case closedCh <- struct{}{}:
				default:
				}
			}()
			var frame machinebridge.Frame
			if err := conn.ReadJSON(&frame); err != nil || frame.Method != machinebridge.MethodHello {
				return
			}
			var hello machinebridge.HelloRequest
			if err := json.Unmarshal(frame.Params, &hello); err != nil {
				return
			}
			mu.Lock()
			connectedHello = append(connectedHello, hello)
			mu.Unlock()
			respBytes, _ := json.Marshal(machinebridge.HelloResponse{
				ProtocolVersion: machinebridge.CurrentProtocolVersion,
				Accepted:        true,
				Capabilities:    machinebridge.Capabilities{FileStaging: true, LocalExec: true, MaxStageBytes: 100 << 20},
			})
			_ = conn.WriteJSON(machinebridge.Frame{ID: frame.ID, Result: respBytes})
			select {
			case connectedCh <- struct{}{}:
			default:
			}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			if strings.HasPrefix(r.URL.Path, "/v1/instructions/") {
				_ = json.NewEncoder(w).Encode(map[string]any{"body_md": "", "version": ""})
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer mockCloud.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	workspace := t.TempDir()
	folderA := filepath.Join(t.TempDir(), "Alpha Repo")
	folderB := filepath.Join(t.TempDir(), "Beta Repo")
	for _, dir := range []string{folderA, folderB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cloud := cloudsync.New(store, nil)
	bridge := NewMachineBridgeSupervisor(cloud, store, workspace, nil)
	bridge.minBackoff = 20 * time.Millisecond
	bridge.maxBackoff = 100 * time.Millisecond
	cloud.AddOnAccountChanged(bridge.Reconcile)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	bridge.Start(runCtx)
	defer bridge.Stop(context.Background())

	if _, err := cloud.Connect(ctx, cloudsync.ConnectRequest{URL: mockCloud.URL, Token: "tok-live"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	waitSignal(t, connectedCh, "initial connect")
	waitCondition(t, func() bool { return bridge.Status().Connected }, "bridge status connected")

	initial := bridge.Status()
	if !initial.Config.Enabled || initial.Config.AllowExec || len(initial.Config.Grants) != 1 || initial.CloudURL != mockCloud.URL || initial.LastConnectedAt == nil {
		t.Fatalf("unexpected initial status: %+v", initial)
	}

	// 1. Live config update with new grants, display name, and LocalExec: true reconnects cleanly.
	updated, err := bridge.UpdateConfig(ctx, machinebridge.ShareConfig{
		Enabled:     true,
		DisplayName: "Studio Mac",
		AllowExec:   true,
		Grants: []machinebridge.FolderGrant{
			{RootPath: folderA, Mode: machinebridge.AccessReadOnly},
			{ID: "beta", RootPath: folderB, Mode: machinebridge.AccessReadWriteDelete},
		},
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if updated.Config.DisplayName != "Studio Mac" || !updated.Config.AllowExec || len(updated.Config.Grants) != 2 || updated.Config.Grants[0].ID != "alpha-repo" {
		t.Fatalf("unexpected updated status: %+v", updated)
	}
	waitSignal(t, closedCh, "disconnect before config reconnect")
	waitSignal(t, connectedCh, "reconnect with updated config")
	waitCondition(t, func() bool { return bridge.Status().Connected }, "reconnected status")

	mu.Lock()
	if len(connectedHello) != 2 {
		t.Fatalf("expected 2 hellos, got %d", len(connectedHello))
	}
	h2 := connectedHello[1]
	mu.Unlock()
	if h2.DisplayName != "Studio Mac" || !h2.Capabilities.LocalExec || !h2.Capabilities.FileStaging || len(h2.FolderGrants) != 2 {
		t.Fatalf("unexpected hello after config update: %+v", h2)
	}

	// 2. Saving identical config does not trigger an unnecessary reconnect.
	if _, err := bridge.UpdateConfig(ctx, updated.Config); err != nil {
		t.Fatalf("no-op UpdateConfig: %v", err)
	}
	select {
	case <-closedCh:
		t.Fatal("no-op config update unexpectedly disconnected bridge")
	case <-time.After(120 * time.Millisecond):
	}

	// 3. Invalid config is rejected without interrupting the active connection.
	if _, err := bridge.UpdateConfig(ctx, machinebridge.ShareConfig{
		Enabled: true,
		Grants:  []machinebridge.FolderGrant{{ID: "bad", RootPath: filepath.Join(workspace, "missing"), Mode: machinebridge.AccessReadOnly}},
	}); !errors.Is(err, machinebridge.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for missing path, got %v", err)
	}
	if !bridge.Status().Connected {
		t.Fatal("invalid update should not drop live connection")
	}

	// 4. Explicit empty grants persists and reconnects with zero grants (never reverts to default workspace).
	emptyStatus, err := bridge.UpdateConfig(ctx, machinebridge.ShareConfig{
		Enabled:     true,
		DisplayName: "Studio Mac",
		AllowExec:   false,
		Grants:      []machinebridge.FolderGrant{},
	})
	if err != nil {
		t.Fatalf("empty grants UpdateConfig: %v", err)
	}
	if len(emptyStatus.Config.Grants) != 0 {
		t.Fatalf("expected empty grants, got %+v", emptyStatus.Config.Grants)
	}
	waitSignal(t, closedCh, "disconnect for empty grants")
	waitSignal(t, connectedCh, "reconnect with empty grants")
	mu.Lock()
	h3 := connectedHello[len(connectedHello)-1]
	mu.Unlock()
	if len(h3.FolderGrants) != 0 || h3.Capabilities.LocalExec {
		t.Fatalf("expected 0 grants and LocalExec=false, got %+v", h3)
	}

	// 5. Setting enabled=false tears down the WSS connection cleanly and stays disconnected.
	paused, err := bridge.UpdateConfig(ctx, machinebridge.ShareConfig{
		Enabled:     false,
		DisplayName: "Studio Mac",
		AllowExec:   false,
		Grants:      []machinebridge.FolderGrant{},
	})
	if err != nil {
		t.Fatalf("disable UpdateConfig: %v", err)
	}
	waitSignal(t, closedCh, "disconnect on disable")
	if paused.Connected || paused.Config.Enabled {
		t.Fatalf("expected paused disconnected status, got %+v", paused)
	}

	// 6. Fresh supervisor loads persisted disabled config and empty grants from SQLite.
	bridge.Stop(context.Background())
	restarted := NewMachineBridgeSupervisor(cloud, store, workspace, nil)
	restarted.minBackoff = 20 * time.Millisecond
	restarted.maxBackoff = 100 * time.Millisecond
	restarted.Start(runCtx)
	defer restarted.Stop(context.Background())

	select {
	case <-connectedCh:
		t.Fatal("disabled bridge unexpectedly connected on restart")
	case <-time.After(150 * time.Millisecond):
	}
	st := restarted.Status()
	if st.DeviceID != initial.DeviceID || st.Config.Enabled || st.Config.DisplayName != "Studio Mac" || len(st.Config.Grants) != 0 {
		t.Fatalf("unexpected persisted status after restart: %+v", st)
	}
}

func waitSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func waitCondition(t *testing.T, cond func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}
