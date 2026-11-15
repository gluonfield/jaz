package oauth

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExistingClientUsesReplacedGrantAndRejectsDeletedGrant(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	defer upstream.Close()
	store := &memoryStore{ok: true, token: Token{AccessToken: "first", TokenType: "Bearer"}}
	client, err := (Refresher{Store: store}).Client(t.Context(), "replacement")
	if err != nil {
		t.Fatal(err)
	}
	for _, access := range []string{"first", "replacement"} {
		store.token.AccessToken = access
		response, err := client.Get(upstream.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(body) != "Bearer "+access {
			t.Fatalf("request used stale grant: %q, %v", body, err)
		}
	}
	store.ok = false
	if _, err := client.Get(upstream.URL); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("deleted grant remains usable: %v", err)
	}
}
