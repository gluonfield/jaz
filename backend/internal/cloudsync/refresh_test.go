package cloudsync_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/cloudsync"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloudSyncSingleFlightRefreshAndRotationPersistence(t *testing.T) {
	ctx := context.Background()
	var (
		mu             sync.Mutex
		activeToken    = "jwt-1"
		activeRefresh  = "rt-1"
		usedRefreshes  = map[string]bool{}
		familyRevoked  bool
		refreshCount   atomic.Int32
		logoutRefresh  string
		logoutToken    string
		logoutDone     = make(chan struct{}, 1)
		refreshStarted = make(chan struct{}, 1)
		releaseRefresh = make(chan struct{})
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
		case "/v1/auth/exchange":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "jwt-1",
				"refresh_token": "rt-1",
				"user":          map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev", "role": "admin"},
			})
		case "/v1/auth/refresh":
			var body struct {
				RefreshToken string `json:"refresh_token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			n := refreshCount.Add(1)
			mu.Lock()
			if familyRevoked || usedRefreshes[body.RefreshToken] || body.RefreshToken != activeRefresh {
				familyRevoked = true
				mu.Unlock()
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "refresh token revoked or reused", "code": "unauthorized"})
				return
			}
			usedRefreshes[body.RefreshToken] = true
			mu.Unlock()

			if n == 1 {
				select {
				case refreshStarted <- struct{}{}:
				default:
				}
				<-releaseRefresh
			}
			mu.Lock()
			activeToken = "jwt-2"
			activeRefresh = "rt-2"
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "jwt-2",
				"refresh_token": "rt-2",
				"user":          map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev Rotated", "role": "admin"},
			})
		case "/v1/auth/logout":
			var body struct {
				RefreshToken string `json:"refresh_token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			logoutToken = r.Header.Get("Authorization")
			logoutRefresh = body.RefreshToken
			mu.Unlock()
			select {
			case logoutDone <- struct{}{}:
			default:
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/v1/auth/me":
			mu.Lock()
			expected := "Bearer " + activeToken
			mu.Unlock()
			if r.Header.Get("Authorization") != expected {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid session token", "code": "unauthorized"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "usr_1", "org_id": "org_1", "org_name": "Example Systems", "email": "u@example.com", "display_name": "Dev", "role": "admin"})
		case "/v1/spaces":
			mu.Lock()
			expected := "Bearer " + activeToken
			mu.Unlock()
			if r.Header.Get("Authorization") != expected {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid session token", "code": "unauthorized"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{{"id": "sp_1", "org_id": "org_1", "name": "Core"}}})
		case "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case "/v1/members":
			mu.Lock()
			expected := "Bearer " + activeToken
			mu.Unlock()
			if r.Header.Get("Authorization") != expected {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid session token", "code": "unauthorized"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"members": []map[string]any{{"id": "usr_1"}}})
		case "/v1/plugins":
			mu.Lock()
			expected := "Bearer " + activeToken
			mu.Unlock()
			if r.Header.Get("Authorization") != expected {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid session token", "code": "unauthorized"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"plugins": []any{}, "sources": []any{}})
		case "/v1/models", "/v1/machines", "/v1/usage/summary", "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	svc := cloudsync.New(store, nil)
	if _, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Code: "code-1", CodeVerifier: "ver-1"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Expire jwt-1 on upstream and fire 4 concurrent callers (proxy + members + spaces + proxy).
	mu.Lock()
	activeToken = "expired-upstream"
	mu.Unlock()

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
			if !svc.ProxyActiveWorkspace(rec, req) || rec.Code != http.StatusOK {
				errs <- context.DeadlineExceeded
			}
		}()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := svc.ListMembers(ctx, "", 10); err != nil {
			errs <- err
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := svc.ListSpaces(ctx); err != nil {
			errs <- err
		}
	}()

	<-refreshStarted
	close(releaseRefresh)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent call after refresh failed: %v", err)
	}

	if got := refreshCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 single-flight refresh, got %d", got)
	}
	mu.Lock()
	revoked := familyRevoked
	mu.Unlock()
	if revoked {
		t.Fatalf("refresh token replay triggered family revocation")
	}
	if _, tok, ok := svc.CloudAuth(ctx); !ok || tok != "jwt-2" {
		t.Fatalf("expected rotated token jwt-2, got ok=%v tok=%q", ok, tok)
	}

	// Reopen SQLite store to confirm rotated token + refresh token persisted encrypted.
	_ = store.Close()
	store2, err := sqlitestore.New(root)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	defer store2.Close()
	svc2 := cloudsync.New(store2, nil)
	if _, tok, ok := svc2.CloudAuth(ctx); !ok || tok != "jwt-2" {
		t.Fatalf("reopened CloudAuth = ok=%v tok=%q", ok, tok)
	}
	if err := svc2.Disconnect(ctx, ""); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	select {
	case <-logoutDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for /v1/auth/logout")
	}
	mu.Lock()
	gotRefresh, gotToken := logoutRefresh, logoutToken
	mu.Unlock()
	if gotRefresh != "rt-2" || gotToken != "Bearer jwt-2" {
		t.Fatalf("unexpected logout credentials: auth=%q refresh=%q", gotToken, gotRefresh)
	}
}

func TestCloudSyncRefresh503PreservesTokensAnd401ClearsSession(t *testing.T) {
	ctx := context.Background()
	var refreshStatus atomic.Int32
	refreshStatus.Store(http.StatusServiceUnavailable)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case "/v1/auth/me":
			if r.Header.Get("Authorization") == "Bearer jwt-init" {
				_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev", "role": "member"})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized", "code": "unauthorized"})
		case "/v1/auth/refresh":
			st := int(refreshStatus.Load())
			w.WriteHeader(st)
			if st == http.StatusUnauthorized {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "refresh token revoked", "code": "unauthorized"})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "unavailable"})
			}
		case "/v1/members":
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid session token", "code": "unauthorized"})
		case "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case "/v1/usage/events":
			if refreshStatus.Load() == http.StatusUnauthorized {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte("{}"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	if _, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Token: "jwt-init", RefreshToken: "rt-init"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	rec := httptest.NewRecorder()
	if !svc.ProxyActiveWorkspace(rec, httptest.NewRequest(http.MethodGet, "/v1/members", nil)) || rec.Code != http.StatusBadGateway {
		t.Fatalf("transient refresh failure must not signal sign-in: %d %s", rec.Code, rec.Body.String())
	}
	if !svc.Status(ctx).Connected {
		t.Fatalf("expected session preserved on transient 503 refresh failure")
	}
	var upstreamErr *cloudsync.UpstreamError
	if _, err := svc.ListMembers(ctx, "", 0); !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("JSON client should report the temporary refresh error, got %v", err)
	}

	refreshStatus.Store(http.StatusUnauthorized)
	st, err := svc.SyncNow(ctx)
	if err == nil || st.Connected || st.LastError != "Session expired. Sign in again." {
		t.Fatalf("a rejected refresh must keep the expiry reason after sync: %+v, %v", st, err)
	}
	if _, tok, ok := svc.CloudAuth(ctx); ok || tok != "" {
		t.Fatalf("expected tokens cleared after 401 refresh failure, got ok=%v tok=%q", ok, tok)
	}
}

func TestCloudRelogIsIndependentOfAnOldRefresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	oldRefreshStarted := make(chan struct{})
	releaseOldRefresh := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/me":
			_, _ = w.Write([]byte(`{"user_id":"user","org_id":"org"}`))
		case "/v1/plugins":
			if r.Header.Get("Authorization") != "Bearer renewed" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"plugins":[]}`))
		case "/v1/auth/refresh":
			var body struct {
				Token string `json:"refresh_token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.Token == "old-refresh" {
				close(oldRefreshStarted)
				<-releaseOldRefresh
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"token":"renewed","refresh_token":"rotated"}`))
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer upstream.Close()
	release := sync.OnceFunc(func() { close(releaseOldRefresh) })
	defer release()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := cloudsync.New(store, nil)
	signIn := func(token string) {
		t.Helper()
		_, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: upstream.URL, Token: token, RefreshToken: token + "-refresh"})
		if err != nil {
			t.Fatal(err)
		}
	}
	requestPlugins := func() int {
		rec := httptest.NewRecorder()
		svc.ProxyActiveWorkspace(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/plugins", nil))
		return rec.Code
	}

	signIn("old")
	oldResult := make(chan int, 1)
	go func() { oldResult <- requestPlugins() }()
	select {
	case <-oldRefreshStarted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	signIn("new")
	if status := requestPlugins(); status != http.StatusOK {
		t.Fatalf("fresh credentials were blocked by old refresh: %d", status)
	}
	release()
	if status := <-oldResult; status != http.StatusOK {
		t.Fatalf("stale refresh failure was reported against the fresh session: %d", status)
	}
	if !svc.Status(ctx).Connected {
		t.Fatal("old refresh failure cleared the new session")
	}
}

func TestCloudSyncDisconnectDuringInFlightRefreshDoesNotResurrectAccount(t *testing.T) {
	ctx := context.Background()
	refreshEntered := make(chan struct{}, 1)
	releaseRefresh := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev", "role": "admin"})
		case "/v1/auth/refresh":
			select {
			case refreshEntered <- struct{}{}:
			default:
			}
			<-releaseRefresh
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "jwt-late",
				"refresh_token": "rt-late",
				"user":          map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev", "role": "admin"},
			})
		case "/v1/auth/logout":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/v1/members":
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid session token", "code": "unauthorized"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	if _, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Token: "jwt-1", RefreshToken: "rt-1"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.ListMembers(ctx, "", 10)
	}()

	<-refreshEntered
	if err := svc.Disconnect(ctx, ""); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	close(releaseRefresh)
	<-done

	accounts, err := store.ListCloudAccounts(ctx)
	if err != nil {
		t.Fatalf("ListCloudAccounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected disconnected account not to be resurrected by late refresh, got %+v", accounts)
	}
}
