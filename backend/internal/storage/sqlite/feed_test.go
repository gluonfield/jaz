package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
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

func assistantReply(t *testing.T, store *Store, id, text string) {
	t.Helper()
	if err := store.AppendSessionEvents(id, sessionevents.Event{Type: "acp_message", Content: text}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFeedTracksUnreadFlag(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-unread"})
	if err != nil {
		t.Fatal(err)
	}
	assistantReply(t, store, session.ID, "done")

	// A reply alone does not surface a thread — only the agent's turn-finish flag.
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("thread should not be in feed before it is flagged unread: %v", ids)
	}

	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}
	items, err := store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != session.ID {
		t.Fatalf("feed = %#v, want the unread thread", items)
	}
	if items[0].ReplyText != "done" {
		t.Fatalf("reply = %q, want 'done'", items[0].ReplyText)
	}

	if err := store.SetThreadUnread(session.ID, false); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("seen thread should leave the feed: %v", ids)
	}
}

func TestLoadFeedUsesLatestReplyAndExcludesArchived(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-last"})
	if err != nil {
		t.Fatal(err)
	}
	assistantReply(t, store, session.ID, "first")
	assistantReply(t, store, session.ID, "latest")
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}

	items, err := store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ReplyText != "latest" {
		t.Fatalf("feed reply = %#v, want newest reply 'latest'", items)
	}

	if err := store.SetArchived(session.ID, true); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("archived thread should be excluded: %v", ids)
	}
}

func TestLoadFeedExcludesSourcedThreads(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// A loop-run thread is automated, not a conversation the user must answer.
	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-loop", SourceType: storage.SourceLoopRun})
	if err != nil {
		t.Fatal(err)
	}
	assistantReply(t, store, session.ID, "loop output")
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("sourced (loop) thread should be excluded: %v", ids)
	}
}
