package sqlite

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestBotRecordAndRoutineOwnershipRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	thread, err := store.CreateSession(storage.CreateSession{Slug: "group", Title: "Launch", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	record := storage.BotRecord{ThreadID: thread.ID, Kind: "group", Prompt: "Keep launch notes.", Workspaces: []string{"/projects/launch"}, Shape: "cloud", Color: "teal", Members: []string{"a", "b"}}
	if err := store.SaveBot(record); err != nil {
		t.Fatal(err)
	}
	if err := store.PinBots([]string{thread.ID}); err != nil {
		t.Fatal(err)
	}
	record.Pinned = 1
	loaded, err := store.LoadBot(thread.ID)
	if err != nil || !reflect.DeepEqual(loaded, record) {
		t.Fatalf("loaded bot = %+v, %v", loaded, err)
	}
	listed, err := store.ListBots()
	if err != nil || !reflect.DeepEqual(listed, []storage.BotRecord{record}) {
		t.Fatalf("listed bots = %+v, %v", listed, err)
	}
	if _, err := store.LoadBot("missing"); !errors.Is(err, storage.ErrBotNotFound) {
		t.Fatalf("missing bot = %v", err)
	}

	service := newLoopServiceForTest(store, nil)
	created, err := service.Create(loops.CreateLoop{Prompt: "ping me", BotID: thread.ID, Trigger: &loops.Trigger{Kind: loops.TriggerWebhook}})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadLoop(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.BotID != thread.ID || stored.Trigger == nil || stored.Trigger.Kind != loops.TriggerWebhook || stored.WebhookHash == "" || stored.WebhookSecret != "" {
		t.Fatalf("stored routine = %+v", stored)
	}
	if !loops.VerifyWebhookSecret(stored, created.WebhookSecret) {
		t.Fatal("the secret returned at creation does not open the stored routine")
	}
}

func TestPinBotsReplacesTheOrderAndSurvivesEdits(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var ids []string
	for _, slug := range []string{"a", "b", "c"} {
		thread, err := store.CreateSession(storage.CreateSession{Slug: slug, Title: slug, SourceType: storage.SourceBot})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveBot(storage.BotRecord{ThreadID: thread.ID, Kind: "bot", Shape: "circle", Color: "blue"}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, thread.ID)
	}
	pins := func() []int {
		var out []int
		for _, id := range ids {
			record, err := store.LoadBot(id)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, record.Pinned)
		}
		return out
	}
	if err := store.PinBots([]string{ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}
	if err := store.PinBots([]string{ids[2], ids[0]}); err != nil {
		t.Fatal(err)
	}
	if got := pins(); !reflect.DeepEqual(got, []int{2, 0, 1}) {
		t.Fatalf("pins after reorder = %v", got)
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: ids[2], Kind: "bot", Shape: "cloud", Color: "teal"}); err != nil {
		t.Fatal(err)
	}
	if got := pins(); !reflect.DeepEqual(got, []int{2, 0, 1}) {
		t.Fatalf("pins after an edit = %v", got)
	}
}

func TestBotMembershipRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var ids []string
	for _, slug := range []string{"group", "bot", "seat"} {
		thread, err := store.CreateSession(storage.CreateSession{Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, thread.ID)
	}
	membership := storage.BotMembership{GroupID: ids[0], BotID: ids[1], ThreadID: ids[2]}
	if err := store.SaveMembership(membership); err != nil {
		t.Fatal(err)
	}
	membership.Seen = 42
	if err := store.SaveMembership(membership); err != nil {
		t.Fatal(err)
	}
	if loaded, err := store.LoadMembership(ids[0], ids[1]); err != nil || loaded != membership {
		t.Fatalf("loaded by group and bot = %+v, %v", loaded, err)
	}
	if loaded, err := store.LoadMembershipByThread(ids[2]); err != nil || loaded != membership {
		t.Fatalf("loaded by thread = %+v, %v", loaded, err)
	}
	if listed, err := store.ListMemberships(); err != nil || !reflect.DeepEqual(listed, []storage.BotMembership{membership}) {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	if _, err := store.LoadMembership(ids[0], ids[2]); !errors.Is(err, storage.ErrMembershipNotFound) {
		t.Fatalf("missing membership = %v", err)
	}
}
