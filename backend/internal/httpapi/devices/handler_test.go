package devices

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestConnectionLinkUsesPublicURL(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := NewHandler(nil, store, serverconfig.Config{Addr: ":5299", PublicURL: "https://jaz.example.com/app"}, "secret")
	res := httptest.NewRecorder()
	handler.ConnectionLink(res, httptest.NewRequest(http.MethodGet, "/v1/devices/connection-link", nil))

	var got connectionLinkResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://jaz.example.com?key=secret" {
		t.Fatalf("url = %q", got.URL)
	}
	if got.BaseURL != "https://jaz.example.com" || got.PublicURL != "" {
		t.Fatalf("connection settings = %#v", got)
	}
}

func TestConnectionLinkFallsBackToRequestHost(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := NewHandler(nil, store, serverconfig.Config{Addr: ":5299"}, "secret")
	req := httptest.NewRequest(http.MethodGet, "/v1/devices/connection-link", nil)
	req.Host = "jaz.example.net:5299"
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httptest.NewRecorder()
	handler.ConnectionLink(res, req)

	var got connectionLinkResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://jaz.example.net:5299?key=secret" {
		t.Fatalf("url = %q", got.URL)
	}
}

func TestUpdateConnectionLinkPersistsPublicURL(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := NewHandler(nil, store, serverconfig.Config{Addr: ":5299", PublicURL: "https://startup.example.com"}, "secret")
	req := httptest.NewRequest(http.MethodPut, "/v1/devices/connection-link", strings.NewReader(`{"public_url":"jaz.example.com"}`))
	res := httptest.NewRecorder()
	handler.UpdateConnectionLink(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var got connectionLinkResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://jaz.example.com?key=secret" || got.BaseURL != "https://jaz.example.com" || got.PublicURL != "https://jaz.example.com" {
		t.Fatalf("connection link = %#v", got)
	}

	getRes := httptest.NewRecorder()
	handler.ConnectionLink(getRes, httptest.NewRequest(http.MethodGet, "/v1/devices/connection-link", nil))
	if err := json.Unmarshal(getRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.PublicURL != "https://jaz.example.com" {
		t.Fatalf("persisted public url = %q", got.PublicURL)
	}

	clearRes := httptest.NewRecorder()
	handler.UpdateConnectionLink(clearRes, httptest.NewRequest(http.MethodPut, "/v1/devices/connection-link", strings.NewReader(`{"public_url":""}`)))
	if err := json.Unmarshal(clearRes.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://startup.example.com?key=secret" || got.BaseURL != "https://startup.example.com" || got.PublicURL != "" {
		t.Fatalf("cleared connection link = %#v", got)
	}
}

func TestRegisterUsesRootPrincipalForApprovedRegistration(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := deviceauth.New(store)
	if _, err := service.Register(testRegistration("Owner", 1)); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/devices/register", strings.NewReader(testDeviceBody("Recovered Mac", 2)))
	req = req.WithContext(deviceauth.WithPrincipal(req.Context(), deviceauth.Principal{Kind: deviceauth.PrincipalRoot}))
	res := httptest.NewRecorder()
	NewHandler(service, store, serverconfig.Config{}, "").Register(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var got struct {
		Token   string      `json:"token"`
		Pairing *pairingDTO `json:"pairing"`
		Device  deviceDTO   `json:"device"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Token == "" || got.Pairing != nil || got.Device.Status != "approved" {
		t.Fatalf("register response = %#v", got)
	}
	if _, err := service.Authenticate(got.Token, deviceauth.SeenInfo{}); err != nil {
		t.Fatalf("token auth: %v", err)
	}
}

func testRegistration(name string, seed byte) deviceauth.Registration {
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
			Name:       name,
			Kind:       "desktop",
			Platform:   "macOS",
			Family:     "Mac",
			Model:      "Mac16,5",
			AppVersion: "0.0.15",
		},
	}
}

func testDeviceBody(name string, seed byte) string {
	reg := testRegistration(name, seed)
	body, err := json.Marshal(map[string]string{
		"name":             reg.Profile.Name,
		"kind":             reg.Profile.Kind,
		"device_id":        reg.Identity.DeviceID,
		"public_key":       reg.Identity.PublicKey,
		"platform":         reg.Profile.Platform,
		"device_family":    reg.Profile.Family,
		"model_identifier": reg.Profile.Model,
		"app_version":      reg.Profile.AppVersion,
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}
