package feed

import (
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

type fakeStore struct {
	items   []storage.FeedItem
	loadErr error
}

func (f *fakeStore) LoadFeed() ([]storage.FeedItem, error) { return f.items, f.loadErr }

func (f *fakeStore) SetThreadUnread(string, bool) error { return nil }

func TestFeedCompactsLastMessage(t *testing.T) {
	store := &fakeStore{items: []storage.FeedItem{{
		ID:    "t1",
		Title: "Hello",
		LastMessage: storage.Message{
			Role:   "assistant",
			Blocks: []storage.Block{{Type: storage.BlockTypeText, Text: "  hi there  "}},
		},
	}}}

	items, err := NewService(store).Feed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].ID != "t1" || items[0].LastMessage.Text != "hi there" {
		t.Fatalf("item = %+v, want compacted text from blocks", items[0])
	}
}

func TestFeedPropagatesLoadError(t *testing.T) {
	want := errors.New("boom")
	if _, err := NewService(&fakeStore{loadErr: want}).Feed(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
