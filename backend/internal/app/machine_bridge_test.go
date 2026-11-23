package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
					Capabilities:    machinebridge.Capabilities{FileStaging: true, MaxStageBytes: 100 << 20},
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

	// 2. Switching to local cleanly disconnects machinebridge without removing Cloud A.
	if _, err := cloud.SwitchWorkspace(ctx, cloudsync.LocalWorkspaceID); err != nil {
		t.Fatalf("SwitchWorkspace local: %v", err)
	}
	select {
	case <-closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for machinebridge disconnect on switch to local")
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
