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
	for _, mode := range []string{"extension", "managed", "desktop"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/browser", strings.NewReader(`{"mode":"`+mode+`"}`)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("obsolete mode %q status=%d", mode, response.Code)
		}
	}
}
