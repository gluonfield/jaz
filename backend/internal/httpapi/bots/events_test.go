package bots

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	botcore "github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBotStreamSnapshotsAndPublishesChangesAcrossThreads(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logger := log.New(io.Discard)
	bus := sessionevents.New()
	manager := acp.NewManager(store, acp.Config{Root: root, Agents: map[string]acp.AgentConfig{"fixture": {Command: "unused"}}}, logger)
	defer manager.Close()
	service := botcore.NewService(store, filepath.Join(root, "bots"), manager, nil, nil, loops.NewService(store, nil, logger), bus, logger)
	handler := NewHandler(service, bus)
	httpServer := httptest.NewServer(http.HandlerFunc(handler.Events))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", httpServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	next := func(kind string, into any) {
		t.Helper()
		if !scanner.Scan() || scanner.Text() != "event: "+kind || !scanner.Scan() {
			t.Fatalf("expected %s, got %q: %v", kind, scanner.Text(), scanner.Err())
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), into); err != nil {
			t.Fatal(err)
		}
		if !scanner.Scan() || scanner.Text() != "" {
			t.Fatal("missing event boundary")
		}
	}
	var initial listResponse
	next("snapshot", &initial)
	if len(initial.Bots) != 0 {
		t.Fatalf("initial = %+v", initial)
	}
	bot, err := service.Create(ctx, botcore.CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	var changed botcore.Bot
	changed = botcore.Bot{}
	next("changed", &changed)
	if changed.ID != bot.ID {
		t.Fatalf("created = %+v", changed)
	}
	if err := service.Pin([]string{bot.ID}); err != nil {
		t.Fatal(err)
	}
	changed = botcore.Bot{}
	next("changed", &changed)
	if changed.Pinned != 1 {
		t.Fatalf("pin = %+v", changed)
	}
	worker, err := store.CreateSession(storage.CreateSession{Slug: "worker", ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Transcript bursts are irrelevant to this projection and cannot overflow it.
	for range 1000 {
		bus.Publish(sessionevents.Event{SessionID: worker.ID, Type: sessionevents.TypeACPThought})
	}
	bus.Publish(sessionevents.Event{SessionID: worker.ID, Type: "permission_request"})
	changed = botcore.Bot{}
	next("changed", &changed)
	if changed.ID != bot.ID {
		t.Fatalf("worker question refreshed %s, want %s", changed.ID, bot.ID)
	}
	peer, err := service.Create(ctx, botcore.CreateBot{Name: "Reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	changed = botcore.Bot{}
	next("changed", &changed)
	group, err := service.CreateGroup("Team", []string{bot.ID, peer.ID})
	if err != nil {
		t.Fatal(err)
	}
	changed = botcore.Bot{}
	next("changed", &changed)
	seat, err := store.CreateSession(storage.CreateSession{Slug: "seat", ParentID: group.ID, SourceType: storage.SourceBotMember, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMembership(storage.BotMembership{GroupID: group.ID, BotID: bot.ID, ThreadID: seat.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(seat.ID, storage.Turn{StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(sessionevents.Event{SessionID: seat.ID, Type: sessionevents.TypeSession})
	changed = botcore.Bot{}
	next("changed", &changed)
	changed = botcore.Bot{}
	next("changed", &changed)
	if changed.ID != group.ID || len(changed.Working) != 1 || changed.Working[0] != bot.ID {
		t.Fatalf("group work = %+v", changed)
	}
	if err := store.FinishSessionTurn(seat.ID, storage.StatusIdle, "", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	bus.Publish(sessionevents.Event{SessionID: seat.ID, Type: sessionevents.TypeSession})
	changed = botcore.Bot{}
	next("changed", &changed)
	changed = botcore.Bot{}
	next("changed", &changed)
	if changed.ID != group.ID || len(changed.Working) != 0 || changed.CompletedAt.IsZero() {
		t.Fatalf("group completion = %+v", changed)
	}
	if err := service.Delete(bot.ID); err != nil {
		t.Fatal(err)
	}
	changed = botcore.Bot{}
	next("changed", &changed)
	if changed.ID != group.ID || len(changed.Members) != 1 || changed.Members[0] != peer.ID {
		t.Fatalf("group after deleting its member = %+v", changed)
	}
	var removed string
	next("removed", &removed)
	if removed != bot.ID {
		t.Fatalf("removed = %q", removed)
	}
}
