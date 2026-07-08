package connections

import (
	"net/http/httptest"
	"testing"
)

func TestCallbackURLPrefersConfiguredBase(t *testing.T) {
	h := NewConnectHandler(nil, nil, nil, nil, "https://jaz.example.com/")
	req := httptest.NewRequest("POST", "http://127.0.0.1:5299/v1/connections/plugins/slack/connect", nil)
	if got := h.callbackURL(req); got != "https://jaz.example.com/v1/connections/oauth/callback" {
		t.Fatalf("callback url = %q", got)
	}
}

func TestCallbackURLFallsBackToRequestHost(t *testing.T) {
	h := NewConnectHandler(nil, nil, nil, nil, "")
	req := httptest.NewRequest("GET", "http://127.0.0.1:5299/x", nil)
	if got := h.callbackURL(req); got != "http://127.0.0.1:5299/v1/connections/oauth/callback" {
		t.Fatalf("callback url = %q", got)
	}
}
