package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestAuthMiddlewareKeepsHealthPublic(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		OK           bool `json:"ok"`
		AuthRequired bool `json:"auth_required"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.AuthRequired {
		t.Fatalf("health = %#v", body)
	}
}

func TestAuthMiddlewareRejectsMissingKey(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsBearerKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsQueryKeyForSessionEvents(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-id/events?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsQueryKeyForRawSessionFile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-id/file?raw=1&path=paper.pdf&key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsQueryKeyForSessionFileJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-id/file?path=paper.pdf&key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsQueryKeyForBrowserExtension(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/browser/extension?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsQueryKeyForWidgetContent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/widgets/widget-id/content?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsQueryKeyOnMutations(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRequiresDeviceAfterBootstrap(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	devices := deviceauth.New(store)
	registered, err := devices.Register(testDeviceInfo("First Mac", "desktop", 1))
	if err != nil {
		t.Fatal(err)
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
	rootReq.Header.Set("Authorization", "Bearer root-key")
	rootRes := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key", Devices: devices}).Handler().ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusForbidden {
		t.Fatalf("root status = %d, body = %s", rootRes.Code, rootRes.Body.String())
	}

	deviceReq := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
	deviceReq.Header.Set("Authorization", "Bearer "+registered.Token)
	deviceRes := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key", Devices: devices}).Handler().ServeHTTP(deviceRes, deviceReq)
	if deviceRes.Code != http.StatusOK {
		t.Fatalf("device status = %d, body = %s", deviceRes.Code, deviceRes.Body.String())
	}
}

func TestAuthMiddlewareAcceptsBrowserExtensionQueryKeyAfterBootstrap(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	devices := deviceauth.New(store)
	if _, err := devices.Register(testDeviceInfo("First Mac", "desktop", 1)); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/browser/extension?key=root-key", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key", Devices: devices}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsLocalBrowserExtensionWithoutKeyAfterBootstrap(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	devices := deviceauth.New(store)
	if _, err := devices.Register(testDeviceInfo("First Mac", "desktop", 1)); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/browser/extension", nil)
	req.RemoteAddr = "127.0.0.1:53124"
	req.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key", Devices: devices}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsWebsiteOriginBrowserExtensionWithoutKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/browser/extension", nil)
	req.RemoteAddr = "127.0.0.1:53124"
	req.Header.Set("Origin", "https://example.com")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsRemoteBrowserExtensionWithoutKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/browser/extension", nil)
	req.RemoteAddr = "203.0.113.10:53124"
	req.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func testDeviceInfo(name, kind string, seed byte) deviceauth.Registration {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed
	}
	sum := sha256.Sum256(raw)
	return deviceauth.Registration{
		Identity: deviceauth.DeviceIdentity{
			DeviceID:  hex.EncodeToString(sum[:]),
			PublicKey: base64.RawURLEncoding.EncodeToString(raw),
		},
		Profile: deviceauth.DeviceProfile{
			Name: name,
			Kind: kind,
		},
	}
}
