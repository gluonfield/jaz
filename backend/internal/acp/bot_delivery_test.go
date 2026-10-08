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

func TestRestartResumesBotAndWorkerAndReturnsResult(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	first := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_BLOCK_PROMPT": "1"}, "", "")
	t.Cleanup(first.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	bot, err := first.Spawn(ctx, acp.SpawnRequest{Slug: "restart-bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Send(ctx, acp.SendRequest{Session: bot.SessionID, Message: "work to resume", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, ctx, first, bot.SessionID)
	worker, err := first.Spawn(ctx, acp.SpawnRequest{ParentID: bot.SessionID, Slug: "restart-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Send(ctx, acp.SendRequest{Session: worker.SessionID, Message: "work to resume", Completion: acp.CompletionAsync, ParentVisible: true}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, ctx, first, worker.SessionID)
	first.Close()
	requestLog := filepath.Join(root, "requests.jsonl")
	second := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{
		"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
	}, "", "")
	t.Cleanup(second.Close)
	storedJob, err := second.Status(worker.SessionID)
	if err != nil || !storedJob.ParentVisible {
		t.Fatalf("interrupted worker disappeared from parent: %+v, %v", storedJob, err)
	}
	locks := sessionlock.New()
	bus := sessionevents.New()
	handler := &server.Server{Store: store, ACP: second, Locks: locks, Events: bus}
	second.TurnFinished = handler.HandleACPTurnFinished
	if err := sessionrecovery.Resume(ctx, store, handler, locks, bus, log.New(io.Discard)); err != nil {
		t.Fatal(err)
	}
	job, err := second.Wait(ctx, acp.WaitRequest{Session: worker.SessionID})
	if err != nil || job.State != acp.StateIdle || !job.ParentVisible {
		t.Fatalf("resumed worker = %+v, %v", job, err)
	}
	for {
		requests, err := os.ReadFile(requestLog)
		if err == nil && strings.Contains(string(requests), "ACP session restart-worker") {
			if strings.Count(string(requests), "ACP session restart-worker") != 1 {
				t.Fatalf("duplicate parent completion: %s", requests)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("resumed worker never returned its result to the parent")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type earlyReplyManager struct {
	*acp.Manager
	speak func(string) error
}

func (m earlyReplyManager) StartInternalTurn(ctx context.Context, req acp.InternalTurnRequest) (acp.Job, error) {
	job, err := m.Manager.StartInternalTurn(ctx, req)
	if err == nil {
		err = m.speak(req.Session)
	}
	return job, err
}

func TestEarlyPeerReplySurvivesServiceAndProviderRestart(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := t.TempDir()
	first := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_BLOCK_PROMPT": "1"}, "", "")
	t.Cleanup(first.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var ids []string
	for _, name := range []string{"Sender", "Recipient"} {
		thread, err := first.CreateSession(ctx, acp.SpawnRequest{Slug: name, Title: name, SourceType: storage.SourceBot, Directory: root})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveBot(storage.BotRecord{ThreadID: thread.ID, Kind: bots.KindBot}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, thread.ID)
	}
	if err := store.StartSessionTurn(ids[0], storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	logger := log.New(io.Discard)
	bus := sessionevents.New()
	handler := &server.Server{Store: store, Locks: sessionlock.New(), Events: bus}
	routines := loops.NewService(store, nil, logger)
	service := bots.NewService(store, root, first, handler, handler, routines, bus, logger)
	spoke := make(chan error, 1)
	handler.ACP = earlyReplyManager{Manager: first, speak: func(id string) error {
		err := service.Say(id, "The complete reply.")
		spoke <- err
		return err
	}}
	first.TurnFinished = handler.HandleACPTurnFinished
	if err := service.Message(ids[0], ids[1], "Find the answer"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-spoke:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("recipient did not start")
	}
	waitForBlockingTool(t, ctx, first, ids[1])
	first.Close()
	second := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_RESUME": "1"}, "", "")
	t.Cleanup(second.Close)
	handler.ACP = second
	second.TurnFinished = handler.HandleACPTurnFinished
	service = bots.NewService(store, root, second, handler, handler, routines, bus, logger)
	if service.AppVisible(ids[1]) {
		t.Fatal("interrupted private reply became publicly visible")
	}
	if err := handler.ResumeInterruptedTurn(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	job, err := second.Wait(ctx, acp.WaitRequest{Session: ids[1]})
	if err != nil || job.State != acp.StateIdle {
		t.Fatalf("resumed peer = %+v, %v", job, err)
	}
	sender, err := store.LoadSession(ids[0])
	if err != nil || len(sender.QueuedMessages) != 1 || !strings.Contains(sender.QueuedMessages[0].Text, "The complete reply.") {
		t.Fatalf("saved peer reply = %+v, %v", sender.QueuedMessages, err)
	}
	events, err := store.LoadSessionEvents(ids[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.RoomMessage != nil {
			t.Fatal("private reply escaped into recipient's public chat")
		}
	}
	if !service.AppVisible(ids[1]) {
		t.Fatal("completed peer turn kept future output private")
	}
}

func waitForBlockingTool(t *testing.T, ctx context.Context, manager *acp.Manager, id string) {
	t.Helper()
	for {
		job, err := manager.Status(id)
		if err != nil {
			t.Fatal(err)
		}
		if len(job.ToolCalls) > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("provider did not start its blocking tool")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
