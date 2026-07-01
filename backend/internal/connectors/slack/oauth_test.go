package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizeURLIncludesPKCEAndScopes(t *testing.T) {
	got := AuthorizeURL("cid", "http://127.0.0.1/cb", "state123", "verifier")
	if !strings.HasPrefix(got, OAuthAuthURL+"?") {
		t.Fatalf("auth url = %s", got)
	}
	for _, want := range []string{"client_id=cid", "code_challenge_method=S256", "code_challenge=", "state=state123", "scope=", "chat%3Awrite"} {
		if !strings.Contains(got, want) {
			t.Fatalf("auth url %s missing %q", got, want)
		}
	}
}

func TestExchangeParsesUserToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.FormValue("code_verifier"); got != "verifier" {
			t.Fatalf("code_verifier = %q", got)
		}
		if r.FormValue("client_id") != "cid" {
			t.Fatalf("client_id = %q", r.FormValue("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"authed_user":{"access_token":"xoxp-1","token_type":"user","scope":"chat:write"}}`))
	}))
	defer srv.Close()

	accessToken, err := exchangeAt(context.Background(), srv.Client(), srv.URL, "cid", "http://127.0.0.1/cb", "code", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if accessToken != "xoxp-1" {
		t.Fatalf("access token = %q", accessToken)
	}
}

func TestExchangeReturnsSlackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
	}))
	defer srv.Close()

	if _, err := exchangeAt(context.Background(), srv.Client(), srv.URL, "cid", "http://127.0.0.1/cb", "bad", "verifier"); err == nil ||
		!strings.Contains(err.Error(), "invalid_code") {
		t.Fatalf("err = %v", err)
	}
}

func TestIdentifyResolvesWorkspace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer xoxp-1" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"ok":true,"team":"Acme","team_id":"T1","user":"augustinas","user_id":"U1"}`))
	}))
	defer srv.Close()

	profile, err := identifyAt(context.Background(), srv.Client(), srv.URL, "xoxp-1")
	if err != nil {
		t.Fatal(err)
	}
	if profile.AccountID() != "T1-U1" || profile.AccountName() != "Acme / augustinas" {
		t.Fatalf("profile = %#v", profile)
	}
}
