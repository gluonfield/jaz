package sqlite

import (
	"context"
	"io/fs"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/wins/jaz/backend/internal/storage"
)

// Threads that a bot's subtasks, group seats and routine runs started before
// they were tagged become the bot's work on upgrade, usage included; every
// other thread keeps its source.
func TestUpgradeGivesTheThreadsASubtaskStartedToItsBot(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	create := func(in storage.CreateSession) string {
		t.Helper()
		session, err := store.CreateSession(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddUsage(session.ID, storage.Usage{InputTokens: 10, OutputTokens: 1}); err != nil {
			t.Fatal(err)
		}
		return session.ID
	}
	bot := create(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	worker := create(storage.CreateSession{Slug: "worker", ParentID: bot, SourceType: storage.SourceBotWorker, SourceID: bot})
	review := create(storage.CreateSession{Slug: "review", ParentID: worker, SourceType: storage.SourceBotWorker, SourceID: bot})
	second := create(storage.CreateSession{Slug: "second-opinion", ParentID: review, SourceType: storage.SourceBotWorker, SourceID: bot})
	search := create(storage.CreateSession{Slug: "search", ParentID: worker, SourceType: storage.SourceMemorySearch, SourceID: "query"})
	seat := create(storage.CreateSession{Slug: "seat", SourceType: storage.SourceBotMember, SourceID: bot})
	errand := create(storage.CreateSession{Slug: "errand", ParentID: seat, SourceType: storage.SourceBotWorker, SourceID: bot})
	run := create(storage.CreateSession{Slug: "run", ParentID: bot, SourceType: storage.SourceLoopRun, SourceID: "loop"})
	report := create(storage.CreateSession{Slug: "report", ParentID: run, SourceType: storage.SourceBotWorker, SourceID: bot})
	chat := create(storage.CreateSession{Slug: "chat"})
	child := create(storage.CreateSession{Slug: "child", ParentID: chat})
	// sources reads the rows themselves, which later migrations may widen
	// beyond what the downgraded schema has.
	slugs := map[string]string{}
	sources := func() (map[string]string, map[string]string) {
		t.Helper()
		threads := map[string]string{}
		for _, id := range []string{bot, worker, review, second, search, seat, errand, run, report, chat, child} {
			var slug, sourceType, sourceID string
			if err := store.db.QueryRow(`SELECT slug, COALESCE(source_type, ''), COALESCE(source_id, '') FROM threads WHERE id = ?`, id).Scan(&slug, &sourceType, &sourceID); err != nil {
				t.Fatal(err)
			}
			slugs[id] = slug
			threads[slug] = sourceType + "/" + sourceID
		}
		events, err := store.UsageEvents(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		usage := map[string]string{}
		for _, event := range events {
			usage[slugs[event.SessionID]] = event.SourceType
		}
		return threads, usage
	}
	upgraded, upgradedUsage := sources()

	migrations, err := fs.Sub(sqliteMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, store.db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(context.Background(), 71); err != nil {
		t.Fatal(err)
	}
	if threads, usage := sources(); threads["review"] != "/" || threads["second-opinion"] != "/" || threads["errand"] != "/" || threads["report"] != "/" || usage["review"] != "" || usage["worker"] != storage.SourceBotWorker {
		t.Fatalf("before the upgrade: threads %v, usage %v; want the subtask's threads untagged as they were", threads, usage)
	}
	if err := store.migrate(); err != nil {
		t.Fatal(err)
	}
	threads, usage := sources()
	want := map[string]string{
		"bot": "bot/", "worker": "bot_worker/" + bot, "review": "bot_worker/" + bot, "second-opinion": "bot_worker/" + bot,
		"search": "memory_search/query", "seat": "bot_member/" + bot, "errand": "bot_worker/" + bot,
		"run": "loop_run/loop", "report": "bot_worker/" + bot, "chat": "/", "child": "/",
	}
	if !maps.Equal(threads, want) || !maps.Equal(threads, upgraded) {
		t.Fatalf("threads after the upgrade = %v, want %v", threads, want)
	}
	if !maps.Equal(usage, upgradedUsage) || usage["second-opinion"] != storage.SourceBotWorker || usage["child"] != "" || usage["search"] != storage.SourceMemorySearch {
		t.Fatalf("usage after the upgrade = %v, want %v", usage, upgradedUsage)
	}
}

// A thread's own children are the threads it started, the bot's work among
// them, but never a bot's own subtasks, routine runs or memory searches.
func TestAThreadsChildrenAreTheThreadsItStarted(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	create := func(in storage.CreateSession) string {
		t.Helper()
		session, err := store.CreateSession(in)
		if err != nil {
			t.Fatal(err)
		}
		return session.ID
	}
	work := func(parent, bot string) storage.CreateSession {
		return storage.CreateSession{ParentID: parent, SourceType: storage.SourceBotWorker, SourceID: bot}
	}
	bot := create(storage.CreateSession{SourceType: storage.SourceBot})
	chat := create(storage.CreateSession{})
	worker := create(work(bot, bot))
	seat := create(storage.CreateSession{SourceType: storage.SourceBotMember, SourceID: bot})
	run := create(storage.CreateSession{ParentID: bot, SourceType: storage.SourceLoopRun, SourceID: "run"})
	want := map[string][]string{
		chat:   {create(storage.CreateSession{ParentID: chat})},
		bot:    nil,
		worker: {create(work(worker, bot))},
		seat:   {create(work(seat, bot))},
		run:    {create(storage.CreateSession{ParentID: run}), create(work(run, bot))},
	}
	create(storage.CreateSession{ParentID: chat, SourceType: storage.SourceMemorySearch, SourceID: "query"})
	create(storage.CreateSession{ParentID: worker, SourceType: storage.SourceMemorySearch, SourceID: "query"})
	for parent, children := range want {
		slices.Sort(children)
		related, err := store.LoadTranscriptSessions(t.Context(), parent, nil)
		if err != nil {
			t.Fatal(err)
		}
		overview, err := store.LoadSessionOverview(t.Context(), parent)
		if err != nil {
			t.Fatal(err)
		}
		var own, listed []string
		for _, child := range related.Children {
			own = append(own, child.ID)
		}
		for _, child := range overview.Threads {
			listed = append(listed, child.ID)
		}
		slices.Sort(own)
		slices.Sort(listed)
		if !slices.Equal(own, children) || !slices.Equal(listed, children) {
			t.Fatalf("children of %s: transcript %v, overview %v; want %v", parent, own, listed, children)
		}
	}
}
