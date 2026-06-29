package sqlite

import (
	"testing"
	"time"

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

func assistantReplyAt(t *testing.T, store *Store, id, text string, atMs int64) {
	t.Helper()
	if err := store.AppendSessionEvents(id, sessionevents.Event{Type: "acp_message", Content: text, At: time.UnixMilli(atMs)}); err != nil {
		t.Fatal(err)
	}
}

func userPromptAt(t *testing.T, store *Store, id, text string, atMs int64) {
	t.Helper()
	if err := store.AppendMessageRecords(id, storage.Message{Role: "user", Content: text, CreatedAt: time.UnixMilli(atMs)}); err != nil {
		t.Fatal(err)
	}
}

func setRunning(t *testing.T, store *Store, id string) {
	t.Helper()
	session, err := store.LoadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusRunning
	if err := store.SaveSession(session); err != nil {
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
	if len(items) != 1 || items[0].ID != session.ID || items[0].ReplyText != "done" {
		t.Fatalf("feed = %#v, want the unread thread with reply 'done'", items)
	}

	if err := store.SetThreadUnread(session.ID, false); err != nil {
		t.Fatal(err)
	}
	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("seen thread should leave the feed: %v", ids)
	}
}

func TestLoadFeedConcatenatesLastTurn(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-turn"})
	if err != nil {
		t.Fatal(err)
	}
	// An earlier turn, then the user prompts again, then a turn split into two
	// reply events around a tool call. Only the latest turn's replies show.
	assistantReplyAt(t, store, session.ID, "earlier turn", 1000)
	userPromptAt(t, store, session.ID, "do it", 2000)
	assistantReplyAt(t, store, session.ID, "working on it", 3000)
	assistantReplyAt(t, store, session.ID, "here is the answer", 4000)
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}

	items, err := store.LoadFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ReplyText != "working on it\n\nhere is the answer" {
		t.Fatalf("reply = %#v, want the whole last turn concatenated", items)
	}
}

func TestLoadFeedExcludesRunningThreads(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-running"})
	if err != nil {
		t.Fatal(err)
	}
	assistantReply(t, store, session.ID, "partial")
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
	}
	setRunning(t, store, session.ID)

	if ids := feedIDs(t, store); contains(ids, session.ID) {
		t.Fatalf("a thread with the agent still working should be excluded: %v", ids)
	}
}

func TestLoadFeedExcludesArchived(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession(storage.CreateSession{Slug: "feed-archived"})
	if err != nil {
		t.Fatal(err)
	}
	assistantReply(t, store, session.ID, "answer")
	if err := store.SetThreadUnread(session.ID, true); err != nil {
		t.Fatal(err)
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
