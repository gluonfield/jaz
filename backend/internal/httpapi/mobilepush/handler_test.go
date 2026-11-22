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

func TestRegistrationRequiresInstallationIDAndIgnoresLegacyDeviceHeader(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sender, err := mobilepush.NewApple(mobilepush.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{Service: mobilepush.NewService(store, sender, func(string) bool { return false })}
	validToken := strings.Repeat("cd", 32)

	// Missing installation_id fails 400 on Register, RegisterActivity, and Unregister even if X-Jaz-Device-ID is sent.
	regReq := httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(`{"token":"`+validToken+`","environment":"development"}`))
	regReq.Header.Set("X-Jaz-Device-ID", "victim-device")
	res := httptest.NewRecorder()
	handler.Register(res, regReq)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing installation_id Register status=%d, want 400", res.Code)
	}

	actReq := httptest.NewRequest(http.MethodPut, "/v1/mobile/push/activities/act-1", strings.NewReader(`{"token":"`+validToken+`","environment":"development","session_id":"sess-1"}`))
	actReq.Header.Set("X-Jaz-Device-ID", "victim-device")
	actReq.SetPathValue("activity", "act-1")
	res = httptest.NewRecorder()
	handler.RegisterActivity(res, actReq)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing installation_id RegisterActivity status=%d, want 400", res.Code)
	}

	unregReq := httptest.NewRequest(http.MethodDelete, "/v1/mobile/push", nil)
	unregReq.Header.Set("X-Jaz-Device-ID", "victim-device")
	res = httptest.NewRecorder()
	handler.Unregister(res, unregReq)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing installation_id Unregister status=%d, want 400", res.Code)
	}

	// Overlong installation_id rejected.
	longID := strings.Repeat("x", 129)
	longReq := httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(`{"token":"`+validToken+`","environment":"development","installation_id":"`+longID+`"}`))
	res = httptest.NewRecorder()
	handler.Register(res, longReq)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("overlong installation_id status=%d, want 400", res.Code)
	}

	// Spoofed X-Jaz-Device-ID cannot overwrite another installation's key or fall back to "default".
	spoofReq := httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(`{"token":"`+validToken+`","environment":"development","installation_id":"install-a"}`))
	spoofReq.Header.Set("X-Jaz-Device-ID", "victim-device")
	res = httptest.NewRecorder()
	handler.Register(res, spoofReq)
	if res.Code != http.StatusNoContent {
		t.Fatalf("valid Register status=%d, want 204", res.Code)
	}
	if _, err := store.LoadSetting("mobile_push", "victim-device"); err == nil {
		t.Fatal("X-Jaz-Device-ID wrote under victim key")
	}
	if _, err := store.LoadSetting("mobile_push", "default"); err == nil {
		t.Fatal("registration wrote under legacy default key")
	}
	if _, err := store.LoadSetting("mobile_push", "install-a"); err != nil {
		t.Fatalf("expected registration under install-a: %v", err)
	}

	okUnreg := httptest.NewRequest(http.MethodDelete, "/v1/mobile/push?installation_id=install-a", nil)
	res = httptest.NewRecorder()
	handler.Unregister(res, okUnreg)
	if res.Code != http.StatusNoContent {
		t.Fatalf("Unregister status=%d, want 204", res.Code)
	}
}
