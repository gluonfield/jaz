package feed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	feedcore "github.com/wins/jaz/backend/internal/feed"
	"github.com/wins/jaz/backend/internal/storage"
)

type fakeStore struct {
	items       []storage.FeedItem
	completions []storage.FeedCompletion
}

func (f fakeStore) LoadFeed() ([]storage.FeedItem, error) { return f.items, nil }

func (f fakeStore) LoadFeedCompletions() ([]storage.FeedCompletion, error) {
	return f.completions, nil
}

func TestListHandlerReturnsItems(t *testing.T) {
	store := fakeStore{items: []storage.FeedItem{{
		ID: "t1", Slug: "alpha", Title: "Alpha", ReplyText: "ping",
	}}}

	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	res := httptest.NewRecorder()
	NewHandler(feedcore.NewService(store)).List(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Items []struct {
			ID          string `json:"id"`
			LastMessage struct {
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"last_message"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != "t1" || got.Items[0].LastMessage.Text != "ping" {
		t.Fatalf("item = %+v", got.Items[0])
	}
}

func TestListHandlerEmptyIsArray(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	res := httptest.NewRecorder()
	NewHandler(feedcore.NewService(fakeStore{})).List(res, req)

	if body := res.Body.String(); body != `{"items":[]}`+"\n" && body != `{"items":[]}` {
		t.Fatalf("empty feed body = %q, want items as []", body)
	}
}

func TestCompletionHandlerReturnsProjection(t *testing.T) {
	at := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/v1/feed/completions", nil)
	res := httptest.NewRecorder()
	NewHandler(feedcore.NewService(fakeStore{completions: []storage.FeedCompletion{{
		ID: "t1", Slug: "alpha", Title: "Alpha", CompletedAt: at,
	}}})).Completions(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Items []feedcore.Completion `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "t1" || !got.Items[0].CompletedAt.Equal(at) {
		t.Fatalf("items = %#v", got.Items)
	}
}
