package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type failingTokenStore struct {
	memoryStore
	saveErr error
}

func (s *failingTokenStore) SaveToken(ctx context.Context, id string, token Token) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.memoryStore.SaveToken(ctx, id, token)
}

func TestFailedRefreshIsSharedUntilGrantReplacement(t *testing.T) {
	for _, mode := range []string{"response_lost", "grant_rejected", "save_lost"} {
		t.Run(mode, func(t *testing.T) {
			var presentations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				presentations.Add(1)
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Form.Get("refresh_token") == "old-refresh" {
					switch mode {
					case "response_lost":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
						return
					case "grant_rejected":
						w.WriteHeader(http.StatusBadRequest)
						_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
						return
					}
				}
				_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
			}))
			defer server.Close()
			store := &failingTokenStore{memoryStore: memoryStore{ok: true, token: Token{
				AccessToken: "expired", RefreshToken: "old-refresh", TokenType: "Bearer",
				Expiry: time.Now().Add(-time.Hour), TokenURL: server.URL,
				AuthStyle: int(oauth2.AuthStyleInParams),
			}}}
			saveErr := errors.New("cannot persist rotated grant")
			if mode == "save_lost" {
				store.saveErr = saveErr
			}
			connectionID := t.Name()
			var guard RefreshGuard
			newSource := func() oauth2.TokenSource {
				t.Helper()
				source, err := (Refresher{Store: store, Guard: &guard}).TokenSource(t.Context(), connectionID)
				if err != nil {
					t.Fatal(err)
				}
				return source
			}
			sources := []oauth2.TokenSource{newSource(), newSource(), newSource(), newSource()}
			errs := make([]error, len(sources))
			var group sync.WaitGroup
			for i, source := range sources {
				group.Go(func() {
					_, errs[i] = source.Token()
				})
			}
			group.Wait()
			_, rebuiltErr := newSource().Token()
			for _, err := range append(errs, rebuiltErr) {
				var failure *RefreshError
				if !errors.As(err, &failure) {
					t.Fatalf("refresh error = %v, want RefreshError", err)
				}
				if mode == "save_lost" && !errors.Is(err, saveErr) {
					t.Fatalf("refresh error lost persistence cause: %v", err)
				}
				if mode == "grant_rejected" {
					var rejected *oauth2.RetrieveError
					if !errors.As(err, &rejected) || rejected.ErrorCode != "invalid_grant" {
						t.Fatalf("refresh error lost rejection code: %v", err)
					}
				}
			}
			if got := presentations.Load(); got != 1 {
				t.Fatalf("failed grant presentations = %d, want 1", got)
			}
			unlock := LockToken(connectionID)
			store.token.Expiry = store.token.Expiry.Add(-time.Hour)
			store.token.ClientID = "updated-client"
			store.token.AccessToken = ""
			unlock()
			if _, err := newSource().Token(); err == nil || presentations.Load() != 1 {
				t.Fatal("metadata change retried the failed grant")
			}
			unlock = LockToken(connectionID)
			store.saveErr = nil
			replacement := store.token
			replacement.AccessToken = "replacement"
			replacement.RefreshToken = "replacement-refresh"
			err := store.SaveToken(t.Context(), connectionID, replacement)
			unlock()
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range append(sources, newSource()) {
				token, err := source.Token()
				if err != nil || token.AccessToken != "fresh" {
					t.Fatalf("replaced grant was not usable: %v", err)
				}
			}
			if got := presentations.Load(); got != 2 {
				t.Fatalf("replacement refresh presentations = %d, want 2 total", got)
			}
			if store.token.RefreshToken != "rotated" {
				t.Fatal("replacement refresh was not persisted")
			}
		})
	}
}

func TestCancelledTokenSourceDoesNotBlockUnusedGrant(t *testing.T) {
	var presentations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		presentations.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	store := &memoryStore{ok: true, token: Token{
		AccessToken: "expired", RefreshToken: "unused", TokenType: "Bearer",
		Expiry: time.Now().Add(-time.Hour), TokenURL: server.URL,
		AuthStyle: int(oauth2.AuthStyleInParams),
	}}
	refresher := Refresher{Store: store, Guard: &RefreshGuard{}}
	ctx, cancel := context.WithCancel(t.Context())
	source, err := refresher.TokenSource(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := source.Token(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled source error = %v", err)
	}
	if presentations.Load() != 0 {
		t.Fatal("cancelled source presented the grant")
	}
	source, err = refresher.TokenSource(t.Context(), t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Token(); err != nil {
		t.Fatalf("unused grant was blocked: %v", err)
	}
	if got := presentations.Load(); got != 1 {
		t.Fatalf("unused grant presentations = %d, want 1", got)
	}
}

func TestRefresherWithoutGuardCanRetry(t *testing.T) {
	var presentations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if presentations.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	store := &memoryStore{ok: true, token: Token{
		AccessToken: "expired", RefreshToken: "reusable", TokenType: "Bearer",
		Expiry: time.Now().Add(-time.Hour), TokenURL: server.URL,
		AuthStyle: int(oauth2.AuthStyleInParams),
	}}
	source, err := (Refresher{Store: store}).TokenSource(t.Context(), t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Token(); err == nil {
		t.Fatal("first refresh succeeded unexpectedly")
	}
	if _, err := source.Token(); err != nil {
		t.Fatalf("unguarded retry failed: %v", err)
	}
	if got := presentations.Load(); got != 2 {
		t.Fatalf("refresh presentations = %d, want 2", got)
	}
}
