package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/deviceauth"
	devicesapi "github.com/wins/jaz/backend/internal/httpapi/devices"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDeviceAdministrationRequiresLocalOwner(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	devices := deviceauth.New(store)
	phone, err := devices.RegisterApproved(testDeviceInfo("Phone", "mobile", 1))
	if err != nil {
		t.Fatal(err)
	}
	pairing, _, err := devices.CreatePairing(testDeviceInfo("Candidate", "desktop", 2))
	if err != nil {
		t.Fatal(err)
	}
	h := devicesapi.NewHandler(devices, store, serverconfig.Config{})
	handler := (&Server{Store: store, Devices: devices, AuthKey: "host-owner", Routes: Routes{
		{Pattern: "GET /v1/devices", Handler: http.HandlerFunc(h.List)},
		{Pattern: "GET /v1/devices/connection-link", Handler: http.HandlerFunc(h.ConnectionLink)},
		{Pattern: "PUT /v1/devices/connection-link", Handler: http.HandlerFunc(h.UpdateConnectionLink)},
		{Pattern: "POST /v1/devices/pairing-requests/{id}/{action}", Handler: http.HandlerFunc(h.Pairing)},
		{Pattern: "DELETE /v1/devices/{id}", Handler: http.HandlerFunc(h.Revoke)},
	}}).Handler()
	for _, credential := range []string{phone.Token, "host-owner"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:5299/v1/devices", nil)
		request.Header.Set("Authorization", "Bearer "+credential)
		request.RemoteAddr = "127.0.0.1:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var list struct {
			CanManage bool `json:"can_manage"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil || list.CanManage != (credential == "host-owner") {
			t.Fatalf("device management capability: %d %s", response.Code, response.Body.String())
		}
	}
	for _, operation := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/devices/connection-link", ""},
		{http.MethodPut, "/v1/devices/connection-link", `{"public_url":"https://untrusted.invalid"}`},
		{http.MethodPost, "/v1/devices/pairing-requests/" + pairing.ID + "/approve", ""},
		{http.MethodPost, "/v1/devices/pairing-requests/" + pairing.ID + "/reject", ""},
		{http.MethodDelete, "/v1/devices/" + phone.Device.ID, ""},
	} {
		for _, credential := range []string{phone.Token, "host-owner"} {
			request := httptest.NewRequest(operation.method, "http://127.0.0.1:5299"+operation.path, strings.NewReader(operation.body))
			request.Header.Set("Authorization", "Bearer "+credential)
			request.RemoteAddr = "127.0.0.1:12345"
			if credential == "host-owner" {
				request.Header.Set("X-Forwarded-Proto", "https")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("non-owner %s %s status=%d", operation.method, operation.path, response.Code)
			}
		}
	}
	loaded, _, err := store.LoadDevicePairing(pairing.ID)
	if err != nil || loaded.Status != "pending" {
		t.Fatalf("denied requests changed pairing: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5299/v1/devices/pairing-requests/"+pairing.ID+"/approve", nil)
	request.Header.Set("Authorization", "Bearer host-owner")
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("host owner could not approve: %d %s", response.Code, response.Body.String())
	}
}

func TestMCPSettingsRedactAndPreserveSavedSecrets(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	handler := (&Server{Store: store}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{"name":"Private tools","url":"https://example.invalid/mcp","bearer_token":"sensitive-bearer","headers":[{"name":"X-Key","value":"sensitive-header"}]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create MCP server: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "sensitive-") {
		t.Fatal("create response exposed credentials")
	}
	request = httptest.NewRequest(http.MethodPut, "/v1/mcp/servers/"+created.ID, strings.NewReader(`{"name":"Renamed tools","url":"https://example.invalid/mcp","headers":[{"name":"X-Key"}]}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	saved, err := store.LoadMCPServer(created.ID)
	if err != nil || response.Code != http.StatusOK || saved.BearerToken != "sensitive-bearer" || len(saved.Headers) != 1 || saved.Headers[0].Value != "sensitive-header" {
		t.Fatalf("editing redacted settings lost credentials: %v, %s", err, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPut, "/v1/mcp/servers/"+created.ID, strings.NewReader(`{"name":"Renamed tools","url":"https://example.invalid/mcp","bearer_token":"","headers":[]}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	saved, err = store.LoadMCPServer(created.ID)
	if err != nil || response.Code != http.StatusOK || saved.BearerToken != "" || len(saved.Headers) != 0 {
		t.Fatalf("explicit credential removal failed: %v, %s", err, response.Body.String())
	}
}
