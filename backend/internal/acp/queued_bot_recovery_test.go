package acp_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/sessionrecovery"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestClaimedBotMessageRecoversBeforeProviderSessionExists(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{
		Slug: "claimed-bot", SourceType: storage.SourceBot,
		RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex, Cwd: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimQueuedTurn(bot, storage.NewInternalQueuedMessage("Saved result for the bot")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	requestLog := filepath.Join(root, "requests.jsonl")
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_REQUEST_LOG": requestLog}, "", "")
	t.Cleanup(manager.Close)
	locks := sessionlock.New()
	bus := sessionevents.New()
	handler := &server.Server{Store: store, ACP: manager, Locks: locks, Events: bus}
	manager.TurnFinished = handler.HandleACPTurnFinished
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := sessionrecovery.Resume(ctx, store, handler, locks, bus, log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: bot.ID})
	if err != nil || job.State != acp.StateIdle {
		t.Fatalf("recovered claim = %+v, %v", job, err)
	}
	requests, err := os.ReadFile(requestLog)
	if err != nil || strings.Count(string(requests), "Saved result for the bot") != 1 || strings.Contains(string(requests), "Continue from where you left off") {
		t.Fatalf("original queued input was lost: %s, %v", requests, err)
	}
}

func TestBotPreservesConfiguredModelAndExplicitSelection(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, nil, "configured-model", "low")
	t.Cleanup(manager.Close)
	bus := sessionevents.New()
	logger := log.New(io.Discard)
	handler := &server.Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: bus}
	service := bots.NewService(store, root, manager, handler, handler, loops.NewService(store, nil, logger), bus, logger)
	for _, model := range []string{"", "explicit-model"} {
		bot, err := service.Create(t.Context(), bots.CreateBot{Name: "Bot " + model, Agent: acp.AgentCodex, Model: model})
		if err != nil {
			t.Fatal(err)
		}
		want := model
		if want == "" {
			want = "configured-model"
		}
		if bot.Model != want || bot.ReasoningEffort != "low" {
			t.Fatalf("bot changed native configuration: %+v", bot)
		}
	}
}
