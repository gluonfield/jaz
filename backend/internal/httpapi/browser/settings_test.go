package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/browsercontrol"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type extensionStatusStub struct {
	status browsercontrol.ExtensionStatus
}

func (s extensionStatusStub) Status() browsercontrol.ExtensionStatus {
	return s.status
}

func TestSettingsEndpoint(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	changed := false
	handler := NewSettingsHandler(store, extensionStatusStub{status: browsercontrol.ExtensionStatus{
		Connected:   true,
		ExtensionID: "ext-1",
		Protocol:    browsercontrol.ExtensionProtocol,
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
	if status.Enabled || status.Mode != jazsettings.BrowserModeExtension {
		t.Fatalf("default browser status = %#v", status)
	}
	if !status.Extension.Connected || status.Extension.ExtensionID != "ext-1" || len(status.Extension.Actions) != 2 {
		t.Fatalf("extension status = %#v", status.Extension)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":true}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("enable browser = %d, body = %s", res.Code, res.Body.String())
	}
	if !changed {
		t.Fatal("settings update did not notify dependencies")
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Mode != jazsettings.BrowserModeExtension {
		t.Fatalf("browser status = %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":false}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("disable browser = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatalf("browser should be disabled, got %#v", status)
	}
}

func TestSettingsEndpointAllowsManagedModeWithoutExtension(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := NewSettingsHandler(store, extensionStatusStub{}, nil)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":true,"mode":"managed"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("set managed browser = %d, body = %s", res.Code, res.Body.String())
	}
	var status StatusResponse
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Mode != jazsettings.BrowserModeManaged {
		t.Fatalf("browser status = %#v", status)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"mode":"extension"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("extension mode without extension should still save = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Mode != jazsettings.BrowserModeExtension {
		t.Fatalf("browser status = %#v", status)
	}
}
