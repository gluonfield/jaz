package httpapi

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedLocalRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5299/", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	if !TrustedLocalRequest(req) {
		t.Fatal("expected direct loopback request to be trusted")
	}

	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	if TrustedLocalRequest(req) {
		t.Fatal("expected forwarded request to be untrusted")
	}

	remote := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5299/", nil)
	remote.RemoteAddr = "192.0.2.10:4321"
	if TrustedLocalRequest(remote) {
		t.Fatal("expected remote IP request to be untrusted")
	}
}

func TestRequestBaseURLUsesForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "internal:5299"
	req.Header.Set("X-Forwarded-Host", "jaz.example.com, proxy.local")
	req.Header.Set("X-Forwarded-Proto", "https")

	if got := RequestBaseURL(req); got != "https://jaz.example.com" {
		t.Fatalf("RequestBaseURL = %q", got)
	}
}

func TestRequestBaseURLFallsBackToTLSHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "jaz.local:5299"
	req.TLS = &tls.ConnectionState{}

	if got := RequestBaseURL(req); got != "https://jaz.local:5299" {
		t.Fatalf("RequestBaseURL = %q", got)
	}
}
