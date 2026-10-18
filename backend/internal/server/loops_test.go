package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

func TestLoopAPIAndManualRun(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	executor := &fakeLoopExecutor{started: make(chan loops.Run, 1)}
	service := newLoopServiceForTest(store, executor)
	srv := &Server{Store: store, Loops: service}

	create := httptest.NewRequest(http.MethodPost, "/v1/loops", strings.NewReader(`{
		"name":"Half hourly",
		"prompt":"check status",
		"schedule":{"kind":"cron","expr":"*/30 * * * *","timezone":"UTC"},
		"runtime":"acp"
	}`))
	create.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, create)
	if res.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}
	var created loops.Loop
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Runtime != loops.RuntimeACP {
		t.Fatalf("created loop = %#v", created)
	}
	wantMemory := filepath.Join(store.RootDir(), "automations", "half-hourly", "memory.md")
	if created.MemoryPath != wantMemory || !strings.Contains(res.Body.String(), `"memory_path"`) {
		t.Fatalf("created memory path = %q, want %q; body = %s", created.MemoryPath, wantMemory, res.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/v1/loops", nil)
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, list)
	var listed struct {
		Loops []loops.Loop `json:"loops"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Loops) != 1 || listed.Loops[0].MemoryPath != wantMemory {
		t.Fatalf("listed loops = %#v, want memory path %q", listed.Loops, wantMemory)
	}

	get := httptest.NewRequest(http.MethodGet, "/v1/loops/"+created.ID, nil)
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, get)
	var detail struct {
		Loop loops.Loop `json:"loop"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Loop.MemoryPath != wantMemory {
		t.Fatalf("detail memory path = %q, want %q", detail.Loop.MemoryPath, wantMemory)
	}

	patch := httptest.NewRequest(http.MethodPatch, "/v1/loops/"+created.ID, strings.NewReader(`{"status":"paused"}`))
	patch.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, patch)
	if res.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", res.Code, res.Body.String())
	}
	var patched loops.Loop
	if err := json.Unmarshal(res.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Status != loops.StatusPaused {
		t.Fatalf("patched status = %q", patched.Status)
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/loops/"+created.ID+"/run", nil)
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, runReq)
	if res.Code != http.StatusOK {
		t.Fatalf("run status = %d, body = %s", res.Code, res.Body.String())
	}
	var run loops.Run
	if err := json.Unmarshal(res.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	select {
	case started := <-executor.started:
		if started.ID != run.ID {
			t.Fatalf("started run = %s, response run = %s", started.ID, run.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("manual run did not dispatch")
	}
}

func TestLoopAPICarriesReasoningEffortAndDirectory(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := &fakeACPManager{spawnStore: store}
	srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: sessionevents.New()}
	service := newLoopServiceForTest(store, NewLoopRunner(srv))
	srv.Loops = service

	create := httptest.NewRequest(http.MethodPost, "/v1/loops", strings.NewReader(`{
		"prompt":"audit the repo",
		"schedule":{"kind":"cron","expr":"0 9 * * *","timezone":"UTC"},
		"runtime":"acp",
		"acp_agent":"codex",
		"reasoning_effort":"high",
		"directory":"sub/dir"
	}`))
	create.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, create)
	if res.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", res.Code, res.Body.String())
	}
	var created loops.Loop
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ReasoningEffort != "high" || created.Directory != "sub/dir" {
		t.Fatalf("created loop = %#v", created)
	}

	get := httptest.NewRequest(http.MethodGet, "/v1/loops/"+created.ID, nil)
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, get)
	var detail struct {
		Loop loops.Loop `json:"loop"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Loop.ReasoningEffort != "high" || detail.Loop.Directory != "sub/dir" {
		t.Fatalf("fetched loop = %#v", detail.Loop)
	}

	run, err := service.RunNow(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitForInternalTurn(t, manager, "audit the repo")
	manager.mu.Lock()
	spawned := manager.spawned
	manager.mu.Unlock()
	if spawned.ReasoningEffort != "high" || spawned.Directory != "sub/dir" {
		t.Fatalf("acp spawn = %#v", spawned)
	}
	if spawned.SourceID != run.ID {
		t.Fatalf("spawn source id = %q, run id = %q", spawned.SourceID, run.ID)
	}
	if !strings.Contains(strings.Join(spawned.SystemPromptExtensions, "\n"), "Scheduled Jaz loop run.") {
		t.Fatalf("spawn system prompt extensions missing loop context:\n%v", spawned.SystemPromptExtensions)
	}
}

func TestACPLoopRunCreatesHiddenThreadAndFinishesFromCallback(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := &fakeACPManager{spawnStore: store}
	srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: sessionevents.New()}
	service := newLoopServiceForTest(store, NewLoopRunner(srv))
	srv.Loops = service
	loop, err := service.Create(loops.CreateLoop{
		Name:     "ACP check",
		Prompt:   "check status",
		Runtime:  loops.RuntimeACP,
		ACPAgent: "codex",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "* * * * *", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}

	run, err := service.RunNow(context.Background(), loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	sent := waitForInternalTurn(t, manager, "check status")
	if sent.Message != "check status" || sent.Output.LoopRun() != run.ID {
		t.Fatalf("run turn = %+v, want the loop task alone, naming its run", sent)
	}
	manager.mu.Lock()
	spawned := manager.spawned
	manager.mu.Unlock()
	if spawned.SourceType != storage.SourceLoopRun || spawned.SourceID != run.ID {
		t.Fatalf("spawn source = %#v", spawned)
	}
	if !strings.Contains(strings.Join(spawned.SystemPromptExtensions, "\n"), "Memory: "+loop.MemoryPath) {
		t.Fatalf("system prompt extensions missing memory path %q:\n%v", loop.MemoryPath, spawned.SystemPromptExtensions)
	}
	session := sessionForRun(t, store, run.ID)
	if session.ID != sent.Session {
		t.Fatalf("run session = %s, sent session = %s", session.ID, sent.Session)
	}
	if session.RuntimeRef == nil {
		t.Fatalf("session runtime ref missing: %#v", session)
	}

	waitForLoopRun(t, service, loop.ID, run.ID, loops.RunStatusRunning)
	if err := store.StartSessionTurn(sent.Session, storage.Turn{Output: sent.Output}); err != nil {
		t.Fatal(err)
	}
	srv.HandleACPTurnFinished(context.Background(), acp.Job{ID: sent.Session, State: acp.StateIdle})
	waitForLoopRun(t, service, loop.ID, run.ID, loops.RunStatusOK)
}

func TestRoutineRunsAsAQueuedTurnInItsBotThread(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bot, err := store.CreateSession(storage.CreateSession{Slug: "gimli", Title: "Gimli", Runtime: storage.RuntimeACP, SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{}
	srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: sessionevents.New()}
	service := newLoopServiceForTest(store, NewLoopRunner(srv))
	srv.Loops = service
	loop, err := service.Create(loops.CreateLoop{
		Name:     "Morning triage",
		Prompt:   "check the inbox",
		BotID:    bot.ID,
		Thread:   loops.ThreadMain,
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "30 8 * * 1-5", Timezone: "UTC"},
	})
	if err != nil {
		t.Fatal(err)
	}

	run, err := service.RunNow(context.Background(), loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	var turn acp.InternalTurnRequest
	waitFor(t, 5*time.Second, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		turn = manager.internal
		return turn.Session != ""
	})
	if turn.Session != bot.ID || turn.Output.LoopRun() != run.ID || !strings.HasPrefix(turn.Message, "[routine] Morning triage · ") || !strings.Contains(turn.Message, "\n\ncheck the inbox\n\nDeliver this routine's outcome") {
		t.Fatalf("routine turn = %+v", turn)
	}
	waitForLoopRun(t, service, loop.ID, run.ID, loops.RunStatusRunning)
	events, err := store.LoadSessionEvents(bot.ID)
	if err != nil || len(events) != 1 || events[0].BotActivity == nil || events[0].BotActivity.Label != "Morning triage" {
		t.Fatalf("bot chat events = %+v, %v", events, err)
	}
	// The manager stores each turn's intent as it starts it; another turn
	// ending in the bot's thread leaves the routine's run alone.
	if err := store.StartSessionTurn(bot.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	srv.HandleACPTurnFinished(context.Background(), acp.Job{ID: bot.ID, State: acp.StateIdle})
	waitForLoopRun(t, service, loop.ID, run.ID, loops.RunStatusRunning)
	if err := store.StartSessionTurn(bot.ID, storage.Turn{Output: turn.Output, AllowSilence: true}); err != nil {
		t.Fatal(err)
	}
	srv.HandleACPTurnFinished(context.Background(), acp.Job{ID: bot.ID, State: acp.StateFailed, Error: "sign-in expired"})
	waitForLoopRun(t, service, loop.ID, run.ID, loops.RunStatusError)
	if manager.spawned.Slug != "" {
		t.Fatalf("a bot routine spawned its own thread: %+v", manager.spawned)
	}
}

type fakeLoopExecutor struct {
	started chan loops.Run
}

func (f *fakeLoopExecutor) StartLoopRun(_ context.Context, execution loops.Execution) {
	f.started <- execution.Run
}

func newLoopServiceForTest(store *sqlitestore.Store, executor loops.Executor) *loops.Service {
	return loops.NewService(store, executor, nil, loops.WithMemoryPaths(loops.NewMemoryPaths(filepath.Join(store.RootDir(), "automations"))))
}

func waitForLoopRun(t *testing.T, service *loops.Service, loopID, runID, status string) {
	t.Helper()
	waitFor(t, time.Second, func() bool {
		runs, err := service.Runs(loopID, 20)
		if err != nil {
			return false
		}
		for _, run := range runs {
			if run.ID == runID {
				return run.Status == status
			}
		}
		return false
	})
}

func waitForInternalTurn(t *testing.T, manager *fakeACPManager, text string) acp.InternalTurnRequest {
	t.Helper()
	var turn acp.InternalTurnRequest
	waitFor(t, time.Second, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		turn = manager.internal
		return strings.Contains(turn.Message, text)
	})
	return turn
}

func sessionForRun(t *testing.T, store *sqlitestore.Store, runID string) storage.Session {
	t.Helper()
	sessions, err := store.ListSessions(storage.SessionFilter{SourceType: storage.SourceLoopRun})
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if session.SourceID == runID {
			return session
		}
	}
	t.Fatalf("session for run %s not found in %#v", runID, sessions)
	return storage.Session{}
}

func TestTurnEndFailsAWidgetRunThatPublishedNothing(t *testing.T) {
	srv, widgetService, store, loop, run := newWidgetLoopFinishTest(t, "thread-unpublished")

	srv.HandleACPTurnFinished(context.Background(), acp.Job{ID: run.ThreadID, State: acp.StateIdle})

	stored, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != loops.RunStatusError || !strings.Contains(stored.Error, "visualise_publish_widget") {
		t.Fatalf("run = %+v", stored)
	}
	widget, _, _, err := widgetService.StateForLoop(loop.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(widget.LastError, "visualise_publish_widget") {
		t.Fatalf("widget last error = %q", widget.LastError)
	}
}

func TestTurnEndFailsAWidgetRunThatOnlyWroteTheFile(t *testing.T) {
	srv, widgetService, store, loop, run := newWidgetLoopFinishTest(t, "thread-file-without-publish")
	path := widgets.WidgetFilePath(loop)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<p>auto</p>"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv.HandleACPTurnFinished(context.Background(), acp.Job{ID: run.ThreadID, State: acp.StateIdle})

	stored, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != loops.RunStatusError || !strings.Contains(stored.Error, "visualise_publish_widget") {
		t.Fatalf("run = %+v", stored)
	}
	state, err := widgetService.RunPublishState(loop.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Published || state.Widget.CurrentVersion != 0 {
		t.Fatalf("publish state = %+v", state)
	}
}

func TestTurnEndAcceptsAWidgetRunThatPublished(t *testing.T) {
	srv, widgetService, store, loop, run := newWidgetLoopFinishTest(t, "thread-published")
	if _, _, err := widgetService.Publish(loop, run.ID, widgets.PublishInput{HTML: "<p>ok</p>"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	srv.HandleACPTurnFinished(context.Background(), acp.Job{ID: run.ThreadID, State: acp.StateIdle})

	stored, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != loops.RunStatusOK || stored.Error != "" {
		t.Fatalf("run = %+v", stored)
	}
}

func newWidgetLoopFinishTest(t *testing.T, threadID string) (*Server, *widgets.Service, *sqlitestore.Store, loops.Loop, loops.Run) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC()
	loop := loops.Loop{
		ID:         store.NewLoopID(),
		Name:       "Widget loop",
		Prompt:     "update",
		Schedule:   loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:     loops.StatusActive,
		Runtime:    loops.RuntimeACP,
		MemoryPath: filepath.Join(t.TempDir(), "widget-loop", "memory.md"),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := store.SaveLoop(loop); err != nil {
		t.Fatal(err)
	}
	runID := store.NewRunID()
	run := loops.Run{
		ID:        runID,
		LoopID:    loop.ID,
		StartedAt: now.Add(-time.Minute),
		Status:    loops.RunStatusRunning,
		CreatedAt: now,
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       threadID,
		Runtime:    storage.RuntimeACP,
		SourceType: storage.SourceLoopRun,
		SourceID:   run.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	run.ThreadID = session.ID
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(session.ID, storage.Turn{Output: &storage.TurnOutput{Run: run.ID}}); err != nil {
		t.Fatal(err)
	}
	widgetService := widgets.NewService(store, nil)
	board, err := widgetService.CreateBoard("Desk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := widgetService.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatal(err)
	}
	loopService := loops.NewService(store, nil, nil, loops.WithRunCheck(widgetService.RequireRunPublished))
	return &Server{Store: store, Loops: loopService, Locks: sessionlock.New(), Events: sessionevents.New()}, widgetService, store, loop, run
}
