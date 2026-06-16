package usage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

func TestDailyHandler(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "usage"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:       12,
		CachedInputTokens: 30,
		OutputTokens:      4,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/usage/daily?days=1&timezone=UTC", nil)
	res := httptest.NewRecorder()

	NewDailyHandler(usagecore.NewService(store)).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Days []storage.DailyUsage `json:"days"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Days) != 1 || got.Days[0].SessionCount != 1 ||
		got.Days[0].Usage.InputTokens != 12 ||
		got.Days[0].Usage.CachedInputTokens != 30 ||
		got.Days[0].Usage.OutputTokens != 4 ||
		got.Days[0].Usage.TotalTokens != 46 {
		t.Fatalf("daily usage = %#v", got.Days)
	}
}

func TestDailyHandlerRejectsInvalidDays(t *testing.T) {
	for _, path := range []string{"/v1/usage/daily?days=-1", "/v1/usage/daily?days=0"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()

		NewDailyHandler(usagecore.NewService(nil)).ServeHTTP(res, req)

		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, body = %s", path, res.Code, res.Body.String())
		}
	}
}
