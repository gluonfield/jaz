package mobilepush

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/mobilepush"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestRegistrationPersistsAuthenticatedPushAndRejectsInvalidUpdates(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sender, err := mobilepush.NewApple(mobilepush.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{Service: mobilepush.NewService(store, sender, func(string) bool {
		return false
	})}
	body := `{"token":"` + strings.Repeat("ab", 32) + `","environment":"development","installation_id":"install-1"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(body))
	res := httptest.NewRecorder()
	handler.Register(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("registration status=%d expected=%d: %s", res.Code, http.StatusNoContent, res.Body.String())
	}
	setting, err := store.LoadSetting("mobile_push", "install-1")
	if err != nil {
		t.Fatal(err)
	}
	var saved mobilepush.Registration
	if err := json.Unmarshal(setting.Value, &saved); err != nil || saved.InstallationID != "install-1" {
		t.Fatalf("registration did not persist: %s %v", setting.Value, err)
	}

	req = httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(`{"token":"not a token","environment":"production","installation_id":"install-1"}`))
	res = httptest.NewRecorder()
	handler.Register(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("malformed token accepted: %d", res.Code)
	}
	after, err := store.LoadSetting("mobile_push", "install-1")
	if err != nil || string(after.Value) != string(setting.Value) {
		t.Fatal("invalid request overwrote working registration")
	}
}
