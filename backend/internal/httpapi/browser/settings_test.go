package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSettingsEndpoint(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	changes := 0
	handler := NewSettingsHandler(store, func() { changes++ })
	for _, enabled := range []bool{true, false} {
		body, err := json.Marshal(settings.BrowserSettings{Enabled: enabled})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(string(body))))
		if response.Code != http.StatusOK {
			t.Fatalf("update status=%d body=%s", response.Code, response.Body.String())
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/browser", nil))
		var status map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || len(status) != 1 || status["enabled"] != enabled {
			t.Fatalf("browser status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if changes != 2 {
		t.Fatalf("change notifications=%d", changes)
	}
	for _, body := range []string{`{}`, `null`, `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":false,"mode":"extension"}`, `{"mode":"managed"}`, `{"mode":"desktop"}`} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid update %s status=%d", body, response.Code)
		}
	}
	current, err := settings.LoadBrowserSettings(store)
	if err != nil || current.Enabled || changes != 2 {
		t.Fatalf("rejected updates changed settings: current=%+v err=%v changes=%d", current, err, changes)
	}
}

func TestSettingsUpdateReplacesInvalidSavedRecord(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.SaveSetting(settings.BrowserSettingsNamespace, settings.BrowserSettingsKey, json.RawMessage(`{"enabled":"invalid"}`)); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewSettingsHandler(store, nil).ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"enabled":true}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", response.Code, response.Body.String())
	}
	current, err := settings.LoadBrowserSettings(store)
	if err != nil || !current.Enabled {
		t.Fatalf("saved settings=%+v err=%v", current, err)
	}
}
