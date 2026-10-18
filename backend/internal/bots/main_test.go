package bots

import (
	"slices"
	"strings"
	"testing"
)

func TestTheMainBotIsCreatedOnceAndOutlivesRenamesAndRestarts(t *testing.T) {
	root := t.TempDir()
	service, store, manager := newConfiguredBotService(t, root)
	research, err := service.Create(t.Context(), CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Pin([]string{research.ID}); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureMain(t.Context()); err != nil {
		t.Fatal(err)
	}
	main := mainBot(t, service)
	if main.Name != "Jaz" || main.Agent != "fixture" || main.Model != "default-model" || main.Pinned != 1 {
		t.Fatalf("main bot = %+v, want Jaz pinned first on the default agent and model", main)
	}
	if research, err = service.Load(research.ID); err != nil || research.Pinned != 2 {
		t.Fatalf("pinned bot = %+v, %v", research, err)
	}
	for _, bot := range []Bot{main, research} {
		session, err := store.LoadSession(bot.ID)
		if err != nil {
			t.Fatal(err)
		}
		modules, err := Prompt(store, session)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(modules, "\n"), mainRole) != bot.Main {
			t.Fatalf("%s's prompt has the main role: %v, want %v", bot.Name, !bot.Main, bot.Main)
		}
	}
	name := "Assistant"
	if _, err := service.Update(t.Context(), main.ID, UpdateBot{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(main.ID); err == nil {
		t.Fatal("the main bot was deleted")
	}

	manager.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	service, store, _ = newConfiguredBotService(t, root)
	// Archiving its thread as a plain thread does not remove it either.
	if err := store.SetArchived(main.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureMain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := mainBot(t, service); after.ID != main.ID || after.Name != "Assistant" {
		t.Fatalf("main bot after a rename, an archive and a restart = %+v, want %s renamed", after, main.ID)
	}
	if bots, err := service.List(); err != nil || len(bots) != 2 {
		t.Fatalf("bots = %+v, %v", bots, err)
	}
}

func mainBot(t *testing.T, service *Service) Bot {
	t.Helper()
	bots, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	bots = slices.DeleteFunc(bots, func(bot Bot) bool { return !bot.Main })
	if len(bots) != 1 {
		t.Fatalf("main bots = %+v, want one", bots)
	}
	return bots[0]
}
