package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browserworker"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type extensionStatusStub struct {
	status browserworker.ExtensionStatus
}

func (s extensionStatusStub) Status() browserworker.ExtensionStatus {
	return s.status
}

func TestSettingsEndpoint(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		acp.AgentCodex: {Enabled: true, Command: "codex-acp"},
	}}); err != nil {
		t.Fatal(err)
	}
	changed := false
	handler := NewSettingsHandler(store, acp.AgentCatalog{
		acp.AgentCodex: {Command: "codex-acp"},
	}, extensionStatusStub{status: browserworker.ExtensionStatus{
		Connected:   true,
		ExtensionID: "ext-1",
		Protocol:    browserworker.ExtensionProtocol,
		BridgeURL:   "ws://127.0.0.1:5299/v1/browser/extension",
		Actions:     []string{"status", "snapshot"},
	}}, func() { changed = true })

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/browser", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var status StatusResponse
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Agent != "" || status.Mode != jazsettings.BrowserModeExtension {
		t.Fatalf("default browser status = %#v", status)
	}
	if !status.Extension.Connected || status.Extension.ExtensionID != "ext-1" || len(status.Extension.Actions) != 2 {
		t.Fatalf("extension status = %#v", status.Extension)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":true,"agent":"codex"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("set browser agent = %d, body = %s", res.Code, res.Body.String())
	}
	if !changed {
		t.Fatal("settings update did not notify dependencies")
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Agent != acp.AgentCodex || !status.Enabled || status.Mode != jazsettings.BrowserModeExtension {
		t.Fatalf("browser status = %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":true}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("enable browser = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Agent != acp.AgentCodex || !status.Enabled {
		t.Fatalf("enable should preserve browser agent, got %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":false}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("disable browser = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Agent != acp.AgentCodex || status.Enabled {
		t.Fatalf("disable should preserve browser agent, got %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"agent":"jaz"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("built-in Jaz browser agent should 400, got %d body = %s", res.Code, res.Body.String())
	}
}

func TestSettingsEndpointAllowsManagedModeWithoutExtension(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		acp.AgentCodex: {Enabled: true, Command: "codex-acp"},
	}}); err != nil {
		t.Fatal(err)
	}
	handler := NewSettingsHandler(store, acp.AgentCatalog{
		acp.AgentCodex: {Command: "codex-acp"},
	}, extensionStatusStub{}, nil)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":true,"agent":"codex","mode":"managed"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("set managed browser = %d, body = %s", res.Code, res.Body.String())
	}
	var status StatusResponse
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Agent != acp.AgentCodex || status.Mode != jazsettings.BrowserModeManaged {
		t.Fatalf("browser status = %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"mode":"extension"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("extension mode without extension should 400, got %d body = %s", res.Code, res.Body.String())
	}
}
