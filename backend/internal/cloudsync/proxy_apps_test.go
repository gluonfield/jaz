package cloudsync_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/cloudsync"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestProxyActiveWorkspaceAppContentAndDraftPassthrough(t *testing.T) {
	ctx := context.Background()
	var (
		seenAuth         string
		seenCookie       string
		seenTicket       string
		seenHeaderTicket string
		seenIfMatch      string
		seenDraftBytes   int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
		case r.URL.Path == "/v1/auth/exchange":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": "cloud-secret-bearer",
				"user":  map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev", "role": "admin"},
			})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "usr_1", "org_id": "org_1", "org_name": "Example Systems", "email": "u@example.com", "display_name": "Dev", "role": "admin"})
		case r.URL.Path == "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case r.URL.Path == "/v1/apps/drafts":
			raw, _ := io.ReadAll(r.Body)
			seenDraftBytes = len(raw)
			seenIfMatch = r.Header.Get("If-Match")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"draft_id": "adraft_1"})
		case strings.HasPrefix(r.URL.Path, "/app-content/"):
			seenAuth = r.Header.Get("Authorization")
			seenCookie = r.Header.Get("Cookie")
			seenTicket = r.URL.Query().Get("ticket")
			seenHeaderTicket = r.Header.Get("X-Jaz-App-Ticket")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Jaz-App-Bundle-SHA256", strings.Repeat("a", 64))
			_, _ = w.Write([]byte("<h1>Sandboxed</h1>"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	svc := cloudsync.New(store, nil)
	if _, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Code: "code-1", CodeVerifier: "ver-1"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 1. Rejects Service-Worker: script
	swRec := httptest.NewRecorder()
	swReq := httptest.NewRequest(http.MethodGet, "/app-content/aaaaaaaaaaaaaaaaaaaaaaaaaa/index.html?ticket=t1", nil)
	swReq.Header.Set("Service-Worker", "script")
	if !svc.ProxyActiveWorkspaceAppContent(swRec, swReq) || swRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for Service-Worker: script, got %d", swRec.Code)
	}

	// 2. Rejects POST on /app-content/*
	postRec := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/app-content/aaaaaaaaaaaaaaaaaaaaaaaaaa/index.html", nil)
	if !svc.ProxyActiveWorkspaceAppContent(postRec, postReq) || postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d", postRec.Code)
	}

	// 3. Forwards ticket without leaking Authorization or cookies and sets fallback CSP
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app-content/aaaaaaaaaaaaaaaaaaaaaaaaaa/index.html?ticket=tok123", nil)
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("X-Jaz-App-Ticket", "hdr-tok")
	if !svc.ProxyActiveWorkspaceAppContent(rec, req) || rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if seenAuth != "" {
		t.Fatalf("expected no Authorization header leaked to app-content upstream, got %q", seenAuth)
	}
	if seenCookie != "" {
		t.Fatalf("expected no Cookie header leaked to app-content upstream, got %q", seenCookie)
	}
	if seenTicket != "tok123" || seenHeaderTicket != "hdr-tok" {
		t.Fatalf("expected ticket forwarding, got query=%q header=%q", seenTicket, seenHeaderTicket)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox allow-scripts allow-forms") {
		t.Fatalf("expected sandbox CSP header, got %q", rec.Header().Get("Content-Security-Policy"))
	}

	// 4. Allows >4 MiB draft uploads and forwards If-Match header on /v1/apps/*
	bigBody := bytes.Repeat([]byte("a"), 5<<20)
	draftRec := httptest.NewRecorder()
	draftReq := httptest.NewRequest(http.MethodPost, "/v1/apps/drafts", bytes.NewReader(bigBody))
	draftReq.Header.Set("Content-Type", "application/zip")
	draftReq.Header.Set("If-Match", `"rev-3"`)
	if !svc.ProxyActiveWorkspace(draftRec, draftReq) || draftRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for 5 MiB draft upload, got %d: %s", draftRec.Code, draftRec.Body.String())
	}
	if seenDraftBytes != len(bigBody) || seenIfMatch != `"rev-3"` {
		t.Fatalf("expected full draft upload and If-Match header, got bytes=%d ifMatch=%q", seenDraftBytes, seenIfMatch)
	}
}
