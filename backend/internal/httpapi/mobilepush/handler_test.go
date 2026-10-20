package mobilepush

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/mobilepush"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestRegistrationUsesAuthenticatedDeviceAndRejectsInvalidUpdates(t *testing.T) {
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
	body := `{"token":"` + strings.Repeat("ab", 32) + `","environment":"development","installation_id":"one","device_id":"victim"}`
	for _, principal := range []deviceauth.Principal{
		{}, {Kind: deviceauth.PrincipalRoot}, {Kind: deviceauth.PrincipalDevice, DeviceID: "phone"},
	} {
		req := httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(body))
		req = req.WithContext(deviceauth.WithPrincipal(context.Background(), principal))
		res := httptest.NewRecorder()
		handler.Register(res, req)
		expected := http.StatusForbidden
		if principal.DeviceID == "phone" {
			expected = http.StatusNoContent
		}
		if res.Code != expected {
			t.Fatalf("registration status=%d expected=%d: %s", res.Code, expected, res.Body.String())
		}
	}
	if _, err := store.LoadSetting("mobile_push", "victim"); !errors.Is(err, storage.ErrSettingNotFound) {
		t.Fatal("request-controlled device ID wrote another device")
	}
	setting, err := store.LoadSetting("mobile_push", "phone")
	if err != nil {
		t.Fatal(err)
	}
	var saved mobilepush.Registration
	if err := json.Unmarshal(setting.Value, &saved); err != nil || saved.InstallationID != "one" {
		t.Fatalf("registration did not persist: %s %v", setting.Value, err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/mobile/push", strings.NewReader(`{"token":"not a token","environment":"production"}`))
	req = req.WithContext(deviceauth.WithPrincipal(context.Background(), deviceauth.Principal{Kind: deviceauth.PrincipalDevice, DeviceID: "phone"}))
	res := httptest.NewRecorder()
	handler.Register(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("malformed token accepted: %d", res.Code)
	}
	after, err := store.LoadSetting("mobile_push", "phone")
	if err != nil || string(after.Value) != string(setting.Value) {
		t.Fatal("invalid request overwrote working registration")
	}
}
