package acp_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/mcpsession"
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
	worker, err := first.Spawn(ctx, acp.SpawnRequest{ParentID: bot.SessionID, Slug: "restart-worker", Commitment: owe(t, store, bot.SessionID)})
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
	speak func(acp.InternalTurnRequest) error
}

func (m earlyReplyManager) StartInternalTurn(ctx context.Context, req acp.InternalTurnRequest) (acp.Job, error) {
	job, err := m.Manager.StartInternalTurn(ctx, req)
	if err == nil {
		err = m.speak(req)
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
	handler.ACP = earlyReplyManager{Manager: first, speak: func(req acp.InternalTurnRequest) error {
		err := service.Say(req.Session, "The complete reply.")
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

func TestThreadABotStartsForAnotherBotReportsBackToThatBotAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	asker, helper := jaz.newBot("Asker"), jaz.newBot("Helper")
	turns := make(chan acp.InternalTurnRequest, 8)
	worker := make(chan string, 1)
	var owed int64
	// The helper hands the asker's request to a thread, then reports what
	// the thread found.
	act := func(p *jazProcess) earlyReplyManager {
		return earlyReplyManager{Manager: p.manager, speak: func(req acp.InternalTurnRequest) error {
			turns <- req
			switch {
			case strings.HasPrefix(req.Message, "[message from Asker]"):
				id, err := p.commit(helper.ID, bots.MCPCommitmentInput{Request: "Build a parser"})
				if err != nil {
					return err
				}
				owed = id
				_, thread, err := acp.NewMCPTools(p.manager).Create(p.ctx, callAs(helper.ID), acp.MCPCreateInput{Prompt: "Write the parser until released", Commitment: id})
				if err != nil {
					return err
				}
				worker <- thread.ThreadID
				return p.bots.Say(req.Session, "On it.")
			case strings.HasPrefix(req.Message, "[result for the bot that asked]"):
				return p.bots.Say(req.Session, "The parser is done.", owed)
			}
			return nil
		}}
	}
	jaz.server.ACP = act(jaz)
	if err := jaz.bots.Message(asker.ID, helper.ID, "Build a parser until released"); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, <-worker)

	jaz.manager.Close()
	jaz = startJaz(t, jaz.ctx, store, root, map[string]string{"JAZ_FAKE_ACP_RESUME": "1"})
	jaz.server.ACP = act(jaz)
	jaz.recover()
	var result acp.InternalTurnRequest
	var answers []acp.InternalTurnRequest
	for len(answers) < 2 {
		select {
		case req := <-turns:
			if strings.HasPrefix(req.Message, "[result for the bot that asked]") {
				result = req
			}
			if req.Session == asker.ID {
				answers = append(answers, req)
			}
		case <-jaz.ctx.Done():
			t.Fatalf("the asker got %+v, want the helper's acknowledgement and the thread's result", answers)
		}
	}
	if result.Session != helper.ID || result.Output == nil || result.Output.ReplyTo != asker.ID || result.Output.Hops != 1 {
		t.Fatalf("result turn = %+v, want the helper answering the asker", result)
	}
	for i, want := range []string{"On it.", "The parser is done."} {
		if answer := answers[i]; !strings.HasPrefix(answer.Message, "[reply from Helper]") || !strings.Contains(answer.Message, want) || answer.Output.Hops != 1 {
			t.Fatalf("answer %d = %+v, want the helper's reply %q one hop from the user", i, answer, want)
		}
	}
	events, err := store.LoadSessionEvents(helper.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.RoomMessage != nil {
			t.Fatalf("the answer for the asker reached the helper's user: %+v", event.RoomMessage)
		}
	}
	if commitment := jaz.commitment(owed); commitment.Requester != asker.ID || commitment.Status != storage.CommitmentDone {
		t.Fatalf("commitment = %+v, want the one owed to the asker done once its answer reached it", commitment)
	}
}

func TestThreadResultReachesTheUserWhenNoBotWaitsForIt(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	asker, helper, other := jaz.newBot("Asker"), jaz.newBot("Helper"), jaz.newBot("Other")
	tools := acp.NewMCPTools(jaz.manager)
	completions := make(chan acp.InternalTurnRequest, 4)
	worker := make(chan string, 1)
	jaz.server.ACP = earlyReplyManager{Manager: jaz.manager, speak: func(req acp.InternalTurnRequest) error {
		switch {
		case req.Session == helper.ID && strings.HasPrefix(req.Message, "[message from Asker]"):
			_, thread, err := tools.Create(jaz.ctx, callAs(helper.ID), acp.MCPCreateInput{Prompt: "Draft the report until released", Commitment: owe(t, store, helper.ID)})
			worker <- thread.ThreadID
			return err
		case req.Session == helper.ID:
			completions <- req
		case req.Session == other.ID:
			// A bot that is not the thread's parent follows it up while it
			// answers a bot itself.
			_, _, err := tools.Send(jaz.ctx, callAs(other.ID), acp.MCPSendInput{ThreadID: <-worker, Prompt: "Add a summary"})
			return err
		}
		return nil
	}}
	if err := jaz.bots.Message(asker.ID, helper.ID, "Draft a report until released"); err != nil {
		t.Fatal(err)
	}
	thread := <-worker
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread)
	// The asker is deleted before the thread it asked for finishes.
	if err := jaz.bots.Delete(asker.ID); err != nil {
		t.Fatal(err)
	}
	jaz.release()
	await := func() acp.InternalTurnRequest {
		select {
		case req := <-completions:
			return req
		case <-jaz.ctx.Done():
			t.Fatal("the thread's result never reached the helper")
		}
		return acp.InternalTurnRequest{}
	}
	results := []acp.InternalTurnRequest{await()}
	worker <- thread
	if err := jaz.bots.Message(helper.ID, other.ID, "Follow up on the report, block until cancelled"); err != nil {
		t.Fatal(err)
	}
	results = append(results, await())
	for i, result := range results {
		if result.Output != nil || !strings.HasPrefix(result.Message, "ACP session") {
			t.Fatalf("result %d = %+v, want a plain report for the helper's user", i, result)
		}
	}
}

// callAs is a tool call made from thread.
func callAs(thread string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(thread)}}
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

func TestAResultForABotDeletedDuringTheAnswerReachesTheUserAfterARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	asker, helper := jaz.newBot("Asker"), jaz.newBot("Helper")
	worker := make(chan string, 1)
	var owed int64
	act := func(p *jazProcess) earlyReplyManager {
		return earlyReplyManager{Manager: p.manager, speak: func(req acp.InternalTurnRequest) error {
			if req.Session != helper.ID {
				return nil
			}
			switch {
			case strings.HasPrefix(req.Message, "[message from Asker]"):
				id, err := p.commit(helper.ID, bots.MCPCommitmentInput{Request: "Build a parser"})
				if err != nil {
					return err
				}
				owed = id
				_, thread, err := acp.NewMCPTools(p.manager).Create(p.ctx, callAs(helper.ID), acp.MCPCreateInput{Prompt: "Write the parser until released", Commitment: id})
				worker <- thread.ThreadID
				return err
			case strings.HasPrefix(req.Message, "[result for the bot that asked]"):
				// The asker goes after its result was routed to it and before
				// the answer reaches it.
				if err := p.bots.Delete(asker.ID); err != nil {
					return err
				}
				return p.bots.Say(req.Session, "The parser is done.", owed)
			case strings.HasPrefix(req.Message, "[undelivered]") && strings.Contains(req.Message, "The parser is done."):
				if commitment := p.commitment(owed); commitment.Status != storage.CommitmentOpen {
					t.Errorf("an answer that never arrived closed its commitment: %+v", commitment)
				}
				return p.bots.Say(req.Session, "Asker left; the parser is done.", owed)
			}
			return nil
		}}
	}
	jaz.server.ACP = act(jaz)
	if err := jaz.bots.Message(asker.ID, helper.ID, "Build a parser until released"); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, <-worker)

	jaz.manager.Close()
	jaz = startJaz(t, jaz.ctx, store, root, map[string]string{"JAZ_FAKE_ACP_RESUME": "1"})
	jaz.server.ACP = act(jaz)
	jaz.recover()
	jaz.eventually("the result reaching the helper's user", func() bool {
		return slices.Contains(jaz.notices(helper.ID), "Asker left; the parser is done.")
	})
	jaz.idle(helper.ID)
	if session, err := store.LoadSession(helper.ID); err != nil || session.Error != "" {
		t.Fatalf("helper = %+v, %v; want its answer's turn to end cleanly", session, err)
	}
	if commitment := jaz.commitment(owed); commitment.Status != storage.CommitmentDone {
		t.Fatalf("commitment = %+v, want done once the user has the answer", commitment)
	}
}

func TestAThreadThatCannotResumeAfterARestartTellsItsBot(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	_, thread, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "audit", Prompt: "Audit the books until released", Commitment: owe(t, store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ThreadID)

	// No agent starts after the restart, so the thread's resume runs out of
	// tries.
	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_EXIT_BEFORE_INIT": "agent unavailable"})
	jaz.eventually("the failure reaching the bot", func() bool {
		for _, message := range jaz.queued(bot.ID) {
			if strings.Contains(message.Text, "ACP session audit") && strings.Contains(message.Text, "failed") {
				return true
			}
		}
		return false
	})
	worker, err := store.LoadSession(thread.ThreadID)
	if err != nil || worker.Status != storage.StatusError || !strings.Contains(worker.Error, "Could not resume chat after restart") {
		t.Fatalf("thread = %+v, %v; want it failed with why", worker, err)
	}
	session, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if note := bots.TurnNote(store)(session); !strings.Contains(note, "thread audit failed") {
		t.Fatalf("the bot's next turn opens with %q, want its thread shown failed", note)
	}
}
