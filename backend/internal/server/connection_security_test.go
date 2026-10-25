package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/deviceauth"
	devicesapi "github.com/wins/jaz/backend/internal/httpapi/devices"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/storage"
	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/terminal"
)

func TestAuthenticatedMCPProtocol(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	devices := deviceauth.New(store)
	if _, err := devices.RegisterApproved(testDeviceInfo("Owner", "desktop", 1)); err != nil {
		t.Fatal(err)
	}
	protocol := mcp.NewServer(&mcp.Implementation{Name: "security-fixture", Version: "1"}, nil)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return protocol
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})
	srv := httptest.NewServer((&Server{Devices: devices, AuthKey: "internal-credential"}).withAuth(handler))
	t.Cleanup(srv.Close)
	for _, credential := range []string{"", "internal-credential"} {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"security-probe","version":"1"}}}`
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp/jaztools", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if credential == "" {
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatal("MCP accepted an unauthenticated protocol client")
			}
			continue
		}
		if res.StatusCode != http.StatusOK || res.Header.Get("Mcp-Session-Id") == "" {
			t.Fatalf("authenticated MCP initialization status=%d", res.StatusCode)
		}
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/mcp/jaztools", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+credential)
		req.Header.Set("Mcp-Session-Id", res.Header.Get("Mcp-Session-Id"))
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
	}
}

func TestSharingUsesScopedSingleUseEnrollment(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	devices := deviceauth.New(store)
	_, err = devices.RegisterApproved(testDeviceInfo("Owner", "desktop", 1))
	if err != nil {
		t.Fatal(err)
	}
	handler := devicesapi.NewHandler(devices, store, serverconfig.Config{PublicURL: "https://jaz.example"})
	srv := httptest.NewServer((&Server{Store: store, Devices: devices, AuthKey: "permanent-root", Routes: Routes{
		{Pattern: "GET /v1/devices/connection-link", Handler: http.HandlerFunc(handler.ConnectionLink)},
		{Pattern: "POST /v1/devices/register", Handler: http.HandlerFunc(handler.Register)},
	}}).Handler())
	t.Cleanup(srv.Close)
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/devices/connection-link", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer permanent-root")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var link struct {
		URL string `json:"url"`
	}
	err = json.NewDecoder(res.Body).Decode(&link)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != http.StatusOK || strings.Contains(link.URL, "permanent-root") {
		t.Fatalf("sharing leaked a recovery credential or failed: %v", err)
	}
	parsed, err := url.Parse(link.URL)
	if err != nil || parsed.RawQuery != "" {
		t.Fatalf("enrollment credential sent to HTTP URL: %v", err)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	invite := fragment.Get("key")
	identity := testDeviceInfo("Phone", "mobile", 2).Identity
	input, err := json.Marshal(map[string]string{"device_id": identity.DeviceID, "public_key": identity.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token  string
		status int
	}{
		{"permanent-root", http.StatusForbidden},
		{invite, http.StatusOK},
		{invite, http.StatusUnauthorized},
	} {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/devices/register", strings.NewReader(string(input)))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "jaz.example"
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("Authorization", "Bearer "+test.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != test.status {
			t.Fatalf("remote enrollment status=%d want=%d", res.StatusCode, test.status)
		}
	}
}

func TestInternalEndpointsRequireCredentialsBehindTunnel(t *testing.T) {
	for _, path := range []string{"/mcp/jaztools", "/mcp/jazmem", "/mcp/proxy", "/v1/browser/extension"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://jaz.example.com"+path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("Origin", "chrome-extension://abcdefghijklmnopabcdefghijklmnop")
			req.Header.Set("X-Forwarded-Proto", "https")
			res := httptest.NewRecorder()
			(&Server{AuthKey: "root"}).Handler().ServeHTTP(res, req)
			if res.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated endpoint status=%d", res.Code)
			}
		})
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5299/mcp/jaztools", strings.NewReader(`[]`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Origin", "https://untrusted.example")
	res := httptest.NewRecorder()
	(&Server{AuthKey: "root"}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || res.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("website-origin request status=%d headers=%v", res.Code, res.Header())
	}
}

func TestResourceTicketsAreBoundedAndRevocable(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	devices := deviceauth.New(store)
	registered, err := devices.Register(testDeviceInfo("Browser", "browser", 1))
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := devices.IssueResourceTicket(registered.Token)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{AuthKey: "root", Devices: devices}
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/v1/bots/events?key=" + registered.Token, http.StatusUnauthorized},
		{"/v1/bots/events?ticket=" + ticket.Token, http.StatusNoContent},
		{"/v1/settings/agents?ticket=" + ticket.Token, http.StatusUnauthorized},
		{"/v1/sessions/test/terminal?ticket=" + ticket.Token, http.StatusUnauthorized},
	} {
		res := httptest.NewRecorder()
		srv.withAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(res, httptest.NewRequest(http.MethodGet, test.path, nil))
		if res.Code != test.status {
			t.Fatalf("%s status=%d want=%d", test.path, res.Code, test.status)
		}
	}
	if _, err := devices.RevokeDevice(registered.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := devices.AuthenticateResourceTicket(ticket.Token); err == nil {
		t.Fatal("revoked device retained resource access")
	}
}

func TestRevocationClosesExistingTerminal(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	devices := deviceauth.New(store)
	_, err = devices.RegisterApproved(testDeviceInfo("Owner", "desktop", 1))
	if err != nil {
		t.Fatal(err)
	}
	phone, err := devices.RegisterApproved(testDeviceInfo("Phone", "mobile", 2))
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "revocation", RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: store.RootDir()}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := terminal.New()
	t.Cleanup(terminals.Close)
	handler := devicesapi.NewHandler(devices, store, serverconfig.Config{})
	srv := httptest.NewServer((&Server{Store: store, AuthKey: "root", Devices: devices, Terminal: terminals, Routes: Routes{
		{Pattern: "DELETE /v1/devices/{id}", Handler: http.HandlerFunc(handler.Revoke)},
	}}).Handler())
	t.Cleanup(srv.Close)
	dialer := websocket.Dialer{Subprotocols: []string{"jaz", "jaz-auth." + phone.Token}}
	conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/sessions/"+session.ID+"/terminal", http.Header{"Origin": {"https://web.jaz.chat"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if response.Header.Get("Sec-WebSocket-Protocol") != "jaz" {
		t.Fatal("upgrade must negotiate jaz without echoing the credential")
	}
	var message terminal.Message
	if err := conn.ReadJSON(&message); err != nil || message.Type != "ready" {
		t.Fatalf("terminal did not become ready: %v", err)
	}
	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/devices/"+phone.Device.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer root")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("revoke status=%d", res.StatusCode)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		err := conn.ReadJSON(&message)
		if err == nil {
			continue
		}
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			t.Fatal("revoked terminal remained connected")
		}
		break
	}
}

func TestPublicClientOriginUsesNormalizedConnectionURL(t *testing.T) {
	srv := &Server{ServerConfig: serverconfig.Config{PublicURL: "https://self-hosted.example/app"}}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://self-hosted.example")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Access-Control-Allow-Origin") != "https://self-hosted.example" {
		t.Fatalf("configured client origin rejected: status=%d", res.Code)
	}
}
