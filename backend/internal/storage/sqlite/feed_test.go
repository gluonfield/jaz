package sqlite

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func feedIDs(t *testing.T, store *Store) []string {
	t.Helper()
	items, err := store.LoadFeed()
	if err != nil {
		t.Fatalf("load feed: %v", err)
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func contains(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func TestLoadFeedSurfacesUnseenThreadsAndSetThreadSeenClears(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-unseen"})
	if err != nil {
		t.Fatal(err)
	}

	// A freshly created thread with no new messages is not in the feed.
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("new empty thread should not be in feed: %v", ids)
	}

	// A message that arrives after creation makes the thread unread.
	if err := store.AppendMessageRecords(session.ID, storage.Message{Role: "assistant", Content: "done"}); err != nil {
		t.Fatal(err)
	}
	items, err := store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != session.ID {
		t.Fatalf("feed = %#v, want the unseen thread", items)
	}
	if items[0].LastMessage.Content != "done" || items[0].LastMessage.Role != "assistant" {
		t.Fatalf("last message = %+v, want assistant/done", items[0].LastMessage)
	}

	// Marking seen clears it from the feed. Append stamps created_at a couple ms
	// ahead of wall-clock, so wait past it before recording the seen mark.
	time.Sleep(5 * time.Millisecond)
	if err := store.SetThreadSeen(session.ID); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("seen thread should leave the feed: %v", ids)
	}
}

func TestLoadFeedUsesLastMessageAndExcludesArchived(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-last"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(session.ID,
		storage.Message{Role: "user", Content: "first"},
		storage.Message{Role: "assistant", Content: "latest"},
	); err != nil {
		t.Fatal(err)
	}

	items, err := store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].LastMessage.Content != "latest" {
		t.Fatalf("feed last message = %#v, want highest-seq 'latest'", items)
	}

	if err := store.SetArchived(session.ID, true); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("archived thread should be excluded: %v", ids)
	}
}

func TestLoadFeedExcludesUserAuthoredLastMessage(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-user-last"})
	if err != nil {
		t.Fatal(err)
	}
	// Last message authored by the user means the thread is waiting on the agent,
	// not on the human, so it must not appear in the feed.
	if err := store.AppendMessageRecords(session.ID,
		storage.Message{Role: "assistant", Content: "answer"},
		storage.Message{Role: "user", Content: "follow-up"},
	); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("thread with a user-authored last message should be excluded: %v", ids)
	}
}
