package cloud_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fakeDesktopLoginStarter struct {
	binPath string
	cleaned atomic.Bool
}

func (f *fakeDesktopLoginStarter) StartIsolatedCloudLogin(agent string) (acp.IsolatedCloudLogin, error) {
	dir, err := os.MkdirTemp("", "jaz-test-cloud-login-*")
	if err != nil {
		return acp.IsolatedCloudLogin{}, err
	}
	return acp.IsolatedCloudLogin{
		Agent: agent,
		Dir:   dir,
		Invocation: acp.AgentLoginInvocation{
			Executable: f.binPath,
			Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
			Available:  true,
		},
		Cleanup: func() {
			f.cleaned.Store(true)
			_ = os.RemoveAll(dir)
		},
	}, nil
}

func TestACPProfileHandlerProxyAndIsolatedDesktopLogin(t *testing.T) {
	var (
		modelsCalls   atomic.Int32
		mu            sync.Mutex
		receivedCreds string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"org_id": "org_1", "org_name": "Acme", "user_id": "u_1", "email": "dev@example.com", "role": "admin",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			n := modelsCalls.Add(1)
			models := []map[string]any{{"profile_id": "vertex-gemini", "provider": "vertex"}}
			if n > 1 {
				models = append(models, map[string]any{"profile_id": "acp:claude", "provider": "claude_oauth", "scope": "user"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/acp/profiles":
			_ = json.NewEncoder(w).Encode(map[string]any{"profiles": []any{}, "supported_agents": []any{}})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/acp/profiles/claude":
			var body struct {
				Files map[string]string `json:"files"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			raw, _ := base64.StdEncoding.DecodeString(body.Files[".credentials.json"])
			mu.Lock()
			receivedCreds = string(raw)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"profile": map[string]any{"agent": "claude", "profile_id": "acp:claude", "status": "connected", "version": 1},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	fakeCLI := filepath.Join(t.TempDir(), "fake-claude")
	script := `#!/bin/sh
echo "Open https://claude.ai/oauth/authorize?code=1 to sign in"
printf '{"claudeAiOauth":{"accessToken":"sk-ant-oat01-fresh","refreshToken":"rt-fresh","expiresAt":1893456000000}}' > "$CLAUDE_CONFIG_DIR/.credentials.json"
`
	if err := os.WriteFile(fakeCLI, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	t.Cleanup(func() { _ = svc.Disconnect(context.Background(), "") })

	starter := &fakeDesktopLoginStarter{binPath: fakeCLI}
	runner := acp.NewCloudLoginRunner(starter.StartIsolatedCloudLogin)
	t.Cleanup(runner.Close)
	acpHandler := cloudapi.NewACPProfileHandler(svc, runner)
	cloudHandler := cloudapi.NewHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", cloudHandler.ListModels)
	mux.HandleFunc("GET /v1/acp/profiles", acpHandler.Proxy)
	mux.HandleFunc("POST /v1/cloud/acp-profiles/{agent}/login", acpHandler.StartDesktopLogin)
	mux.HandleFunc("GET /v1/cloud/acp-profiles/{agent}/login/{id}", acpHandler.GetDesktopLogin)

	// Local mode rejects Cloud ACP proxy and desktop login with 401.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/acp/profiles", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 in local mode, got %d", rec.Code)
	}

	if _, err := svc.Connect(context.Background(), cloudsync.ConnectRequest{
		URL:   upstream.URL,
		Token: "tok-1",
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Prime /v1/models cache.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "acp:claude") {
		t.Fatalf("unexpected initial models: %d %s", rec.Code, rec.Body.String())
	}

	// Start isolated desktop login for Claude.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/acp-profiles/claude/login", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("StartDesktopLogin status %d: %s", rec.Code, rec.Body.String())
	}
	var started struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil || started.Session.ID == "" {
		t.Fatalf("unexpected login start payload: %s", rec.Body.String())
	}

	// Poll until completed.
	deadline := time.Now().Add(5 * time.Second)
	var finalStatus, finalURL string
	for time.Now().Before(deadline) {
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/cloud/acp-profiles/claude/login/"+started.Session.ID, nil))
		var polled struct {
			Session struct {
				Status          string `json:"status"`
				VerificationURL string `json:"verification_url"`
				Error           string `json:"error"`
			} `json:"session"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &polled)
		finalStatus = polled.Session.Status
		finalURL = polled.Session.VerificationURL
		if finalStatus == "completed" || finalStatus == "failed" {
			if finalStatus == "failed" {
				t.Fatalf("isolated login failed: %s", polled.Session.Error)
			}
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if finalStatus != "completed" || !strings.Contains(finalURL, "claude.ai/oauth") {
		t.Fatalf("expected completed login with OAuth URL, got status=%q url=%q", finalStatus, finalURL)
	}
	mu.Lock()
	gotCreds := receivedCreds
	mu.Unlock()
	if !strings.Contains(gotCreds, "sk-ant-oat01-fresh") {
		t.Fatalf("expected uploaded fresh credentials, got %q", gotCreds)
	}
	if !starter.cleaned.Load() {
		t.Fatal("expected isolated login temp dir cleanup to run")
	}

	// /v1/models cache was invalidated so acp:claude appears immediately.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "acp:claude") {
		t.Fatalf("expected /v1/models cache invalidation to expose acp:claude, got %d %s", rec.Code, rec.Body.String())
	}

	// Switching away from the workspace mid-login must block cross-workspace upload.
	wsID := svc.ActiveWorkspaceID()
	if _, err := svc.SwitchWorkspace(context.Background(), cloudsync.LocalWorkspaceID); err != nil {
		t.Fatalf("SwitchWorkspace: %v", err)
	}
	if _, _, err := svc.PutACPProfileForWorkspace(context.Background(), wsID, "claude", []byte(`{"files":{}}`)); err != cloudsync.ErrWorkspaceChanged {
		t.Fatalf("expected ErrWorkspaceChanged after switching workspaces, got %v", err)
	}
}
