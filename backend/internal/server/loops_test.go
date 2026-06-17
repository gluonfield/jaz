package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
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
	service := newLoopServiceForTest(store, NewLoopRunner(&Server{Store: store, ACP: manager}))
	srv := &Server{Store: store, Loops: service, ACP: manager}

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
	waitForACPSendContaining(t, manager, "You are running a scheduled Jaz loop.")
	manager.mu.Lock()
	spawned := manager.spawned
	manager.mu.Unlock()
	if spawned.ReasoningEffort != "high" || spawned.Directory != "sub/dir" {
		t.Fatalf("acp spawn = %#v", spawned)
	}
	if spawned.SourceID != run.ID {
		t.Fatalf("spawn source id = %q, run id = %q", spawned.SourceID, run.ID)
	}
}

func TestACPLoopRunCreatesHiddenThreadAndFinishesFromCallback(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := &fakeACPManager{spawnStore: store}
	srv := &Server{Store: store, ACP: manager}
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
	sent := waitForACPSendContaining(t, manager, "You are running a scheduled Jaz loop.")
	if sent.Session == "" {
		t.Fatalf("missing sent session: %#v", sent)
	}
	if sent.Interactive {
		t.Fatalf("loop run must be autonomous, got interactive send: %#v", sent)
	}
	if !strings.Contains(sent.Message, "Memory file: "+loop.MemoryPath) {
		t.Fatalf("acp loop prompt missing memory path %q:\n%s", loop.MemoryPath, sent.Message)
	}
	manager.mu.Lock()
	spawned := manager.spawned
	manager.mu.Unlock()
	if spawned.SourceType != storage.SourceLoopRun || spawned.SourceID != run.ID {
		t.Fatalf("spawn source = %#v", spawned)
	}
	session := sessionForRun(t, store, run.ID)
	if session.ID != sent.Session {
		t.Fatalf("run session = %s, sent session = %s", session.ID, sent.Session)
	}

	if _, ok, err := service.FinishThread(sent.Session, loops.RunStatusOK, ""); err != nil || !ok {
		t.Fatalf("finish loop run from acp callback = ok %v, err %v", ok, err)
	}
	waitForLoopRun(t, service, loop.ID, run.ID, loops.RunStatusOK)
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

func waitForACPSendContaining(t *testing.T, manager *fakeACPManager, text string) acp.SendRequest {
	t.Helper()
	var sent acp.SendRequest
	waitFor(t, time.Second, func() bool {
		sent = sentACPRequest(manager)
		return strings.Contains(sent.Message, text)
	})
	return sent
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
