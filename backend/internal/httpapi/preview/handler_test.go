package preview

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/serverconfig"
)

const testCapabilityID = "0123456789abcdef0123456789abcdef"

func TestCreateProxyUsesConfiguredPreviewOrigin(t *testing.T) {
	handler := newHandler(t, "https://app-{id}.preview.example.test")
	got := createProxy(t, handler, "jaz.example.test:5299", "http://localhost:3000/dashboard?tab=1")
	parsed := mustParseURL(t, got.URL)
	id := strings.TrimSuffix(strings.TrimPrefix(parsed.Hostname(), "app-"), ".preview.example.test")
	if len(id) != 32 || !publicHost(handler, parsed.Host) {
		t.Fatalf("preview host = %q", parsed.Host)
	}
	if parsed.Scheme != "https" || parsed.Host != "app-"+id+".preview.example.test" || parsed.RequestURI() != "/dashboard?tab=1" {
		t.Fatalf("url = %q", got.URL)
	}
}

func TestCreateProxyRequiresConfiguredOriginForRemoteBackend(t *testing.T) {
	handler := newHandler(t, "")
	res := registerProxy(handler, "jaz.example.test", "http://localhost:3000/")
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), "--preview-url-template") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestCreateProxyKeepsImplicitLocalElectronOrigin(t *testing.T) {
	handler := newHandler(t, "")
	got := createProxy(t, handler, "127.0.0.1:5299", "http://localhost:3000/dashboard?tab=1")
	parsed := mustParseURL(t, got.URL)
	if parsed.Scheme != "http" || !strings.HasPrefix(parsed.Host, "jaz-preview-") || !strings.HasSuffix(parsed.Host, ".localhost:5299") || !publicHost(handler, parsed.Host) {
		t.Fatalf("url = %q", got.URL)
	}
}

func TestNewHandlerValidatesPreviewOriginTemplate(t *testing.T) {
	for _, template := range []string{
		"https://preview.example.test",
		"https://preview-{id}.example.test/path",
		"file://preview-{id}.example.test",
		"https://preview.example.test/{id}",
		"https://preview_{id}.example.test",
	} {
		t.Run(template, func(t *testing.T) {
			if _, err := NewHandler(serverconfig.Config{PreviewURLTemplate: template}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCreateProxyReusesOriginCapability(t *testing.T) {
	handler := newHandler(t, "https://{id}.preview.example.test")
	first := createProxy(t, handler, "jaz.example.test", "http://localhost:3000/dashboard")
	second := createProxy(t, handler, "jaz.example.test", "http://localhost:3000/settings")
	if mustParseURL(t, first.URL).Host != mustParseURL(t, second.URL).Host {
		t.Fatalf("ids differ: first=%q second=%q", first.URL, second.URL)
	}
}

func TestHostProxyPreservesRootRelativePaths(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer upstream.Close()

	handler := newHandler(t, "https://{id}.preview.example.test")
	proxy := createProxy(t, handler, "jaz.example.test", upstream.URL+"/dashboard")
	for _, path := range []string{"/dashboard", "/client/route", "/assets/app.js", "/assets/app.css"} {
		res := requestPreview(handler, proxy.URL, path)
		if res.Code != http.StatusOK || res.Body.String() != path {
			t.Fatalf("%s status = %d, body = %s", path, res.Code, res.Body.String())
		}
	}
}

func TestProxyStripsJazAndBrowserCredentials(t *testing.T) {
	gotHeaders := make(http.Header)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	handler := newHandler(t, "https://{id}.preview.example.test")
	addProxy(handler, testCapabilityID, testCapabilityID+".preview.example.test", mustParseURL(t, upstream.URL), time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Host = testCapabilityID + ".preview.example.test"
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Proxy-Authorization", "Bearer proxy-secret")
	req.Header.Set("Cookie", "jaz=secret")
	req.Header.Set("Origin", "https://jaz.example.test")
	req.Header.Set("Referer", "https://jaz.example.test/thread")
	req.Header.Set("X-Jaz-Client-Platform", "mobile")
	req.Header.Set("X-Jaz-Device-ID", "device-secret")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK || res.Body.String() != "ok" {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Origin", "Referer", "X-Jaz-Client-Platform", "X-Jaz-Device-ID"} {
		if got := gotHeaders.Get(name); got != "" {
			t.Fatalf("forwarded %s = %q", name, got)
		}
	}
}

func TestHostProxyRewritesLoopbackRedirect(t *testing.T) {
	var upstreamURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", upstreamURL+"/login")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	upstreamURL = upstream.URL
	handler := newHandler(t, "https://{id}.preview.example.test")
	addProxy(handler, testCapabilityID, testCapabilityID+".preview.example.test", mustParseURL(t, upstream.URL), time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = testCapabilityID + ".preview.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if got := res.Header().Get("Location"); got != "https://"+testCapabilityID+".preview.example.test/login" {
		t.Fatalf("location = %q", got)
	}
}

func TestHostProxyForwardsWebSocketWithoutCredentials(t *testing.T) {
	var gotOrigin, gotAuthorization, gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		gotAuthorization = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		messageType, message, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(messageType, message)
		}
	}))
	defer upstream.Close()

	handler := newHandler(t, "http://{id}.preview.example.test")
	addProxy(handler, testCapabilityID, testCapabilityID+".preview.example.test", mustParseURL(t, upstream.URL), time.Now().Add(time.Hour))
	frontend := httptest.NewServer(handler)
	defer frontend.Close()
	dialer := websocket.Dialer{NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("tcp", frontend.Listener.Addr().String())
	}}
	headers := http.Header{
		"Authorization": {"Bearer secret"},
		"Cookie":        {"jaz=secret"},
		"Origin":        {"https://jaz.example.test"},
	}
	conn, response, err := dialer.Dial("ws://"+testCapabilityID+".preview.example.test/@vite/client", headers)
	if err != nil {
		if response != nil {
			t.Fatalf("dial status = %d, err = %v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("hmr")); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil || string(message) != "hmr" {
		t.Fatalf("message = %q, err = %v", message, err)
	}
	if gotOrigin != "" || gotAuthorization != "" || gotCookie != "" {
		t.Fatalf("forwarded websocket credentials origin=%q authorization=%q cookie=%q", gotOrigin, gotAuthorization, gotCookie)
	}
}

func TestPreviewProbeRequiresLiveCapability(t *testing.T) {
	handler := newHandler(t, "https://{id}.preview.example.test")
	addProxy(handler, testCapabilityID, testCapabilityID+".preview.example.test", mustParseURL(t, "http://localhost:3000"), time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, probePath, nil)
	req.Host = testCapabilityID + ".preview.example.test"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || res.Header().Get("X-Jaz-Preview") != "ready" || res.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("status = %d, headers = %#v", res.Code, res.Header())
	}

	req.Host = strings.Repeat("f", 32) + ".preview.example.test"
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("unknown capability status = %d", res.Code)
	}
}

func TestExpiredCapabilityIsRemoved(t *testing.T) {
	handler := newHandler(t, "https://{id}.preview.example.test")
	origin := mustParseURL(t, "http://localhost:3000")
	addProxy(handler, testCapabilityID, testCapabilityID+".preview.example.test", origin, time.Now().Add(-time.Second))

	res := requestPreview(handler, "https://"+testCapabilityID+".preview.example.test/", "/")
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if handler.proxiesByOrigin[origin.String()].id != "" || handler.proxiesByHost[testCapabilityID+".preview.example.test"].id != "" {
		t.Fatal("expired capability was not pruned")
	}
}

func TestProxyRejectsPublicOrigins(t *testing.T) {
	handler := newHandler(t, "https://{id}.preview.example.test")
	res := registerProxy(handler, "jaz.example.test", "https://example.com")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestHostPreviewForwardsMethodsThroughCapability(t *testing.T) {
	var gotMethod string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	handler := newHandler(t, "https://{id}.preview.example.test")
	addProxy(handler, testCapabilityID, testCapabilityID+".preview.example.test", mustParseURL(t, upstream.URL), time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = testCapabilityID + ".preview.example.test"
	if !handler.IsPublicHostRequest(req) {
		t.Fatal("capability host was not public")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || gotMethod != http.MethodPost {
		t.Fatalf("status = %d, upstream method = %q", res.Code, gotMethod)
	}

	req.Host = "preview-not-a-capability.preview.example.test"
	if handler.IsPublicHostRequest(req) {
		t.Fatal("invalid preview host was public")
	}
	req.Host = "jaz.example.test"
	req.URL.Path = "/v1/preview/p/" + testCapabilityID + "/"
	if handler.IsPublicHostRequest(req) {
		t.Fatal("same-origin preview path was public")
	}
}

func newHandler(t *testing.T, template string) *Handler {
	t.Helper()
	handler, err := NewHandler(serverconfig.Config{PreviewURLTemplate: template})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func addProxy(handler *Handler, id, host string, origin *url.URL, expiresAt time.Time) {
	host = canonicalHost(host)
	proxy := proxyEntry{id: id, origin: origin, host: host, expiresAt: expiresAt}
	handler.proxiesByOrigin[origin.String()] = proxy
	handler.proxiesByHost[host] = proxy
}

func publicHost(handler *Handler, host string) bool {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = host
	return handler.IsPublicHostRequest(req)
}

func createProxy(t *testing.T, handler *Handler, requestHost, rawURL string) createProxyResponse {
	t.Helper()
	res := registerProxy(handler, requestHost, rawURL)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got createProxyResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func registerProxy(handler *Handler, requestHost, rawURL string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(createProxyRequest{URL: rawURL})
	req := httptest.NewRequest(http.MethodPost, "/v1/preview/proxies", strings.NewReader(string(body)))
	req.Host = requestHost
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func requestPreview(handler *Handler, source, path string) *httptest.ResponseRecorder {
	parsed, _ := url.Parse(source)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = parsed.Host
	if parsed.Scheme == "https" {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
