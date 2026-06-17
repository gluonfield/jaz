package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestInfoFromUsesForwardedIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.8, 10.0.0.1")
	req.Header.Set("User-Agent", " JazDesktop/1 ")

	got := RequestInfoFrom(req)
	if got.IP != "203.0.113.8" {
		t.Fatalf("IP = %q, want forwarded client IP", got.IP)
	}
	if got.UserAgent != "JazDesktop/1" {
		t.Fatalf("UserAgent = %q, want trimmed user agent", got.UserAgent)
	}
}

func TestRequestIPFallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4321"

	if got := requestIP(req); got != "192.0.2.10" {
		t.Fatalf("requestIP = %q, want remote address host", got)
	}
}
