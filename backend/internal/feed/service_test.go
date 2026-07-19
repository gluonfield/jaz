package feed

import (
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type fakeStore struct {
	items       []storage.FeedItem
	completions []storage.FeedCompletion
	loadErr     error
}

func (f *fakeStore) LoadFeed() ([]storage.FeedItem, error) { return f.items, f.loadErr }

func (f *fakeStore) LoadFeedCompletions() ([]storage.FeedCompletion, error) {
	return f.completions, f.loadErr
}

func TestFeedBuildsAssistantReply(t *testing.T) {
	store := &fakeStore{items: []storage.FeedItem{{ID: "t1", Title: "Hello", ReplyText: "hi there"}}}

	items, err := NewService(store).Feed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].ID != "t1" || items[0].LastMessage.Role != "assistant" || items[0].LastMessage.Text != "hi there" {
		t.Fatalf("item = %+v, want the reply as an assistant message", items[0])
	}
}

func TestCompletionsBuildsLightweightProjection(t *testing.T) {
	at := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{completions: []storage.FeedCompletion{{
		ID: "t1", Slug: "hello", Title: "Hello", CompletedAt: at,
	}}}

	items, err := NewService(store).Completions()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "t1" || !items[0].CompletedAt.Equal(at) {
		t.Fatalf("items = %#v", items)
	}
}

func TestFeedPropagatesLoadError(t *testing.T) {
	want := errors.New("boom")
	if _, err := NewService(&fakeStore{loadErr: want}).Feed(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
