package preview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCreateProxyReturnsHostPreviewURL(t *testing.T) {
	handler := NewHandler()
	body := strings.NewReader(`{"url":"http://localhost:3000/dashboard?tab=1"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/preview/proxies", body)
	req.Host = "jaz.example.test:5299"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got createProxyResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.URL, "http://jaz-preview-") || !strings.Contains(got.URL, ".jaz.example.test:5299/dashboard?tab=1") {
		t.Fatalf("url = %q", got.URL)
	}
	if !strings.Contains(got.FallbackURL, "/v1/preview/p/") || !strings.HasSuffix(got.FallbackURL, "/dashboard?tab=1") {
		t.Fatalf("fallback_url = %q", got.FallbackURL)
	}
}

func TestCreateProxyUsesLocalhostPreviewHostForLoopbackBackend(t *testing.T) {
	handler := NewHandler()
	body := strings.NewReader(`{"url":"http://localhost:3000/dashboard?tab=1"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/preview/proxies", body)
	req.Host = "127.0.0.1:5299"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got createProxyResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.URL, "http://jaz-preview-") || !strings.Contains(got.URL, ".localhost:5299/dashboard?tab=1") {
		t.Fatalf("url = %q", got.URL)
	}
}

func TestCreateProxyReusesOriginCapability(t *testing.T) {
	handler := NewHandler()
	first := createProxy(t, handler, "http://localhost:3000/dashboard")
	second := createProxy(t, handler, "http://localhost:3000/settings")

	if hostPreviewID(first.URL) != hostPreviewID(second.URL) {
		t.Fatalf("ids differ: first=%q second=%q", first.URL, second.URL)
	}
}

func TestHostProxyForwardsRootRelativeAssets(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("asset"))
	}))
	defer upstream.Close()
	target := mustParseURL(t, upstream.URL)
	handler := NewHandler()
	handler.proxies["abc123"] = proxyEntry{origin: target, expiresAt: time.Now().Add(time.Hour)}

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Host = "jaz-preview-abc123.localhost:5299"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK || res.Body.String() != "asset" {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if gotPath != "/assets/app.js" {
		t.Fatalf("upstream path = %q", gotPath)
	}
}

func TestProxyPreviewForwardsLoopbackWithoutJazCredentials(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotCookie, gotReferer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		gotReferer = r.Header.Get("Referer")
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	handler := NewHandler()
	handler.proxies["abc123"] = proxyEntry{origin: mustParseURL(t, upstream.URL), expiresAt: time.Now().Add(time.Hour)}
	req := httptest.NewRequest(http.MethodGet, "/v1/preview/p/abc123/assets/app.js?x=1&key=secret", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Cookie", "jaz=secret")
	req.Header.Set("Referer", "https://jaz.example/v1/preview/p/abc123/")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK || res.Body.String() != "ok" {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if gotPath != "/assets/app.js" || gotQuery != "x=1&key=secret" {
		t.Fatalf("upstream target = %s?%s", gotPath, gotQuery)
	}
	if gotAuth != "" || gotCookie != "" || gotReferer != "" {
		t.Fatalf("forwarded credentials auth=%q cookie=%q referer=%q", gotAuth, gotCookie, gotReferer)
	}
}

func TestProxyPreviewRejectsPublicOrigins(t *testing.T) {
	res := httptest.NewRecorder()
	NewHandler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/preview/proxies", strings.NewReader(`{"url":"https://example.com"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func createProxy(t *testing.T, handler *Handler, rawURL string) createProxyResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/preview/proxies", strings.NewReader(`{"url":"`+rawURL+`"}`))
	req.Host = "jaz.example.test:5299"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got createProxyResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func hostPreviewID(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	id, _ := HostID(parsed.Host)
	return id
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
