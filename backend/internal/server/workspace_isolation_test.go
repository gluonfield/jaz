package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/boards"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/feed"
	botsapi "github.com/wins/jaz/backend/internal/httpapi/bots"
	feedapi "github.com/wins/jaz/backend/internal/httpapi/feed"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/loops"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/threads"
)

type switchableCloudBridge struct {
	active string
}

func (c *switchableCloudBridge) ActiveWorkspaceID() string {
	if c.active == "" {
		return storage.LocalWorkspaceID
	}
	return c.active
}

func (c *switchableCloudBridge) GetSession(context.Context, string) (storage.Session, bool, error) {
	return storage.Session{}, false, nil
}

func (c *switchableCloudBridge) CreateThread(context.Context, cloudsync.CreateThreadRequest) (storage.Session, error) {
	return storage.Session{}, nil
}

func (c *switchableCloudBridge) StreamEvents(context.Context, string, int64, func(sessionevents.Event)) error {
	return nil
}

func (c *switchableCloudBridge) UpdateUserState(context.Context, string, storage.CloudThreadUserStatePatch) (storage.Session, json.RawMessage, error) {
	return storage.Session{}, nil, nil
}

func (c *switchableCloudBridge) CancelThread(context.Context, string) (json.RawMessage, int, error) {
	return json.RawMessage(`{"canceled":true}`), 200, nil
}

func (c *switchableCloudBridge) ListSessions(context.Context, bool) ([]storage.Session, error) {
	return nil, nil
}

type workspaceBotThreads struct {
	store *sqlitestore.Store
	homes string
}

func (t workspaceBotThreads) CreateSession(_ context.Context, req acp.SpawnRequest) (storage.Session, error) {
	dir := filepath.Join(t.homes, req.Slug)
	_ = os.MkdirAll(dir, 0o755)
	return t.store.CreateSession(storage.CreateSession{
		Slug:        req.Slug,
		Title:       req.Title,
		Runtime:     storage.RuntimeACP,
		SourceType:  req.SourceType,
		SourceID:    req.SourceID,
		WorkspaceID: req.WorkspaceID,
		RuntimeRef:  &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "codex", Cwd: dir},
	})
}

func (workspaceBotThreads) SteerInternal(context.Context, string, string, bool) (acp.Job, error) {
	return acp.Job{}, nil
}

func (workspaceBotThreads) SwitchAgent(context.Context, string, string) error { return nil }
func (workspaceBotThreads) SetModel(context.Context, string, string, string) error {
	return nil
}
func (workspaceBotThreads) AwaitsAnswer(string) bool { return false }
func (workspaceBotThreads) Thinking(string) bool     { return false }

type workspaceMCPRuntime struct {
	store     *sqlitestore.Store
	workspace func() string
}

func (r workspaceMCPRuntime) ListMCPServers() ([]mcpconfig.Server, error) {
	return r.store.ListMCPServersForWorkspace(r.workspace())
}
func (workspaceMCPRuntime) Refresh(context.Context) {}
func (workspaceMCPRuntime) RefreshServer(context.Context, string) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{Status: "connected"}
}
func (workspaceMCPRuntime) Status(string) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{Status: "connected"}
}
func (workspaceMCPRuntime) Test(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{Status: "connected"}
}
func (workspaceMCPRuntime) Authorize(context.Context, mcpconfig.Server, mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{Status: "connected"}
}

func TestWorkspaceHTTPHandlerIsolation(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	logger := log.New(io.Discard)
	cloud := &switchableCloudBridge{active: storage.LocalWorkspaceID}
	events := sessionevents.New()
	homes := filepath.Join(root, "bots")

	appSvc := apps.NewService(store, store, nil, nil, root, root)
	boardSvc := boards.NewService(store, cloud.ActiveWorkspaceID, logger).WithApps(appSvc)
	loopSvc := loops.NewService(store, nil, logger, loops.WithWorkspace(cloud.ActiveWorkspaceID))
	inst := instructions.NewLocalFSStore(root, instructions.WithBotsRoot(homes))
	srv := &Server{
		Store:   store,
		Cloud:   cloud,
		Events:  events,
		Loops:   loopSvc,
		Boards:  boardSvc,
		Threads: threads.NewService(sqlitestore.NewSearchQueries(store), store).WithWorkspace(cloud.ActiveWorkspaceID),
		MCP:     workspaceMCPRuntime{store: store, workspace: cloud.ActiveWorkspaceID},
		Root:    root,
		Log:     logger,
	}
	botSvc := bots.NewService(store, homes, inst, workspaceBotThreads{store: store, homes: homes}, srv, srv, loopSvc, events, logger).WithWorkspace(cloud.ActiveWorkspaceID)
	srv.RoutineOwner = botSvc.RoutineOwner

	botsHandler := botsapi.NewHandler(botSvc, events)
	listHandler := sessionsapi.NewListHandler(store).WithCloud(cloud)
	feedHandler := feedapi.NewHandler(feed.NewService(store).WithWorkspace(cloud.ActiveWorkspaceID))
	srv.Routes = Routes{
		{Pattern: "GET /v1/sessions", Handler: listHandler},
		{Pattern: "GET /v1/feed", Handler: http.HandlerFunc(feedHandler.List)},
		{Pattern: "GET /v1/bots", Handler: http.HandlerFunc(botsHandler.List)},
		{Pattern: "POST /v1/bots", Handler: http.HandlerFunc(botsHandler.Create)},
		{Pattern: "GET /v1/bots/{bot}", Handler: http.HandlerFunc(botsHandler.Get)},
		{Pattern: "DELETE /v1/bots/{bot}", Handler: http.HandlerFunc(botsHandler.Delete)},
		{Pattern: "PUT /v1/bots/pins", Handler: http.HandlerFunc(botsHandler.Pin)},
	}
	httpHandler := srv.Handler()

	doJSON := func(method, target string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		var body io.Reader
		if payload != nil {
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(raw)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, target, body)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		httpHandler.ServeHTTP(rec, req)
		return rec
	}

	// --- 1. Bots isolation ---
	if err := botSvc.EnsureMain(t.Context()); err != nil {
		t.Fatalf("EnsureMain: %v", err)
	}
	rec := doJSON(http.MethodPost, "/v1/bots", map[string]any{"name": "LocalOps"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create local bot status=%d body=%s", rec.Code, rec.Body.String())
	}
	var localBot bots.Bot
	_ = json.Unmarshal(rec.Body.Bytes(), &localBot)

	cloud.active = "cloudA"
	rec = doJSON(http.MethodGet, "/v1/bots", nil)
	var botList struct {
		Bots []bots.Bot `json:"bots"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &botList)
	if len(botList.Bots) != 0 {
		t.Fatalf("cloudA leaked local bots: %+v", botList.Bots)
	}
	if cross := doJSON(http.MethodGet, "/v1/bots/"+localBot.ID, nil); cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace GET bot status=%d, want 404", cross.Code)
	}
	if cross := doJSON(http.MethodDelete, "/v1/bots/"+localBot.ID, nil); cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace DELETE bot status=%d, want 404", cross.Code)
	}
	rec = doJSON(http.MethodPost, "/v1/bots", map[string]any{"name": "CloudAOps"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create cloudA bot status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cloudABot bots.Bot
	_ = json.Unmarshal(rec.Body.Bytes(), &cloudABot)
	if cloudABot.WorkspaceID != "cloudA" {
		t.Fatalf("cloudA bot workspace_id = %q, want cloudA", cloudABot.WorkspaceID)
	}
	if sess, err := store.LoadSession(cloudABot.ID); err != nil || sess.WorkspaceID != "cloudA" {
		t.Fatalf("cloudA bot thread workspace_id = %q, err = %v", sess.WorkspaceID, err)
	}
	cloud.active = "cloudB"
	rec = doJSON(http.MethodGet, "/v1/bots", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &botList)
	if len(botList.Bots) != 0 {
		t.Fatalf("cloudB leaked bots: %+v", botList.Bots)
	}

	// --- 2. Boards and Loops isolation ---
	cloud.active = storage.LocalWorkspaceID
	localBoard, err := appSvc.CreateBoard(t.Context(), "Local Board")
	if err != nil {
		t.Fatal(err)
	}
	rec = doJSON(http.MethodPost, "/v1/loops", map[string]any{
		"name":      "Local Loop",
		"prompt":    "check local",
		"schedule":  map[string]any{"kind": "cron", "expr": "0 * * * *", "timezone": "UTC"},
		"board_ids": []string{localBoard.ID},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create local loop status=%d body=%s", rec.Code, rec.Body.String())
	}
	var localLoop loops.Loop
	_ = json.Unmarshal(rec.Body.Bytes(), &localLoop)

	cloud.active = "cloudA"
	var loopList struct {
		Loops []loops.Loop `json:"loops"`
	}
	rec = doJSON(http.MethodGet, "/v1/loops", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &loopList)
	if len(loopList.Loops) != 0 {
		t.Fatalf("cloudA leaked local loops: %+v", loopList.Loops)
	}
	if cross := doJSON(http.MethodGet, "/v1/loops/"+localLoop.ID, nil); cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace GET loop status=%d, want 404", cross.Code)
	}
	if cross := doJSON(http.MethodDelete, "/v1/loops/"+localLoop.ID, nil); cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace DELETE loop status=%d, want 404", cross.Code)
	}
	cloudALoopBody := map[string]any{
		"name":      "CloudA Loop",
		"prompt":    "check cloudA",
		"schedule":  map[string]any{"kind": "cron", "expr": "0 * * * *", "timezone": "UTC"},
		"board_ids": []string{localBoard.ID},
	}
	if cross := doJSON(http.MethodPost, "/v1/loops", cloudALoopBody); cross.Code != http.StatusBadRequest {
		t.Fatalf("cloudA loop on a local board status=%d, want 400", cross.Code)
	}
	delete(cloudALoopBody, "board_ids")
	rec = doJSON(http.MethodPost, "/v1/loops", cloudALoopBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("create cloudA loop status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cloudALoop loops.Loop
	_ = json.Unmarshal(rec.Body.Bytes(), &cloudALoop)
	if spawn := loopRunSpawn(cloudALoop); spawn.WorkspaceID != "cloudA" {
		t.Fatalf("loopRunSpawn workspace_id = %q, want cloudA", spawn.WorkspaceID)
	}

	// --- 3. Custom MCP Servers isolation ---
	cloud.active = storage.LocalWorkspaceID
	rec = doJSON(http.MethodPost, "/v1/mcp/servers", map[string]any{
		"name": "LocalTool", "url": "https://local-mcp.example.com", "enabled": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create local mcp status=%d body=%s", rec.Code, rec.Body.String())
	}
	var localMCP struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &localMCP)

	cloud.active = "cloudA"
	rec = doJSON(http.MethodGet, "/v1/mcp/servers", nil)
	var mcpList struct {
		Servers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"servers"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mcpList)
	if len(mcpList.Servers) != 0 {
		t.Fatalf("cloudA leaked local mcp servers: %+v", mcpList.Servers)
	}
	if cross := doJSON(http.MethodDelete, "/v1/mcp/servers/"+localMCP.ID, nil); cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace DELETE mcp server status=%d, want 404", cross.Code)
	}
	if cross := doJSON(http.MethodPost, "/v1/mcp/servers/"+localMCP.ID+"/disable", nil); cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace disable mcp server status=%d, want 404", cross.Code)
	}

	// --- 4. Projects isolation ---
	dirLocal := t.TempDir()
	dirCloudA := t.TempDir()
	cloud.active = storage.LocalWorkspaceID
	if rec := doJSON(http.MethodPost, "/v1/projects", map[string]any{"path": dirLocal}); rec.Code != http.StatusOK {
		t.Fatalf("add local project status=%d body=%s", rec.Code, rec.Body.String())
	}
	cloud.active = "cloudA"
	var projList struct {
		Projects []project `json:"projects"`
	}
	rec = doJSON(http.MethodGet, "/v1/projects", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &projList)
	if len(projList.Projects) != 0 {
		t.Fatalf("cloudA leaked local projects: %+v", projList.Projects)
	}
	if rec := doJSON(http.MethodPost, "/v1/projects", map[string]any{"path": dirCloudA}); rec.Code != http.StatusOK {
		t.Fatalf("add cloudA project status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(http.MethodGet, "/v1/projects", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &projList)
	if len(projList.Projects) != 1 || projList.Projects[0].Path != dirCloudA {
		t.Fatalf("cloudA projects = %+v", projList.Projects)
	}
	cloud.active = "cloudB"
	rec = doJSON(http.MethodGet, "/v1/projects", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &projList)
	if len(projList.Projects) != 0 {
		t.Fatalf("cloudB leaked projects: %+v", projList.Projects)
	}

	// --- 5. Search, Feed, and Archived sessions isolation ---
	now := time.Now().UTC()
	sLocal, err := store.CreateSession(storage.CreateSession{Slug: "s-local", Title: "AlphaSearch Local", WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	sCloudA, err := store.CreateSession(storage.CreateSession{Slug: "s-a", Title: "AlphaSearch CloudA", WorkspaceID: "cloudA"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []storage.Session{sLocal, sCloudA} {
		_ = store.AppendSessionEvents(s.ID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "hello AlphaSearch", At: now})
		_ = store.CompleteSession(s.ID, now)
	}

	cloud.active = "cloudA"
	var searchResp struct {
		Results []struct {
			ThreadID string `json:"thread_id"`
		} `json:"results"`
	}
	rec = doJSON(http.MethodGet, "/v1/search/threads?q=AlphaSearch", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &searchResp)
	if len(searchResp.Results) != 1 || searchResp.Results[0].ThreadID != sCloudA.ID {
		t.Fatalf("cloudA search = %+v", searchResp.Results)
	}
	var feedResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	rec = doJSON(http.MethodGet, "/v1/feed", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &feedResp)
	if len(feedResp.Items) != 1 || feedResp.Items[0].ID != sCloudA.ID {
		t.Fatalf("cloudA feed = %+v", feedResp.Items)
	}

	_ = store.SetArchived(sLocal.ID, true)
	_ = store.SetArchived(sCloudA.ID, true)
	var sessionsResp struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	rec = doJSON(http.MethodGet, "/v1/sessions?archived=true", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &sessionsResp)
	if len(sessionsResp.Sessions) != 1 || sessionsResp.Sessions[0].ID != sCloudA.ID {
		t.Fatalf("cloudA archived sessions = %+v", sessionsResp.Sessions)
	}
	cloud.active = "cloudB"
	rec = doJSON(http.MethodGet, "/v1/sessions?archived=true", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &sessionsResp)
	if len(sessionsResp.Sessions) != 0 {
		t.Fatalf("cloudB leaked archived sessions = %+v", sessionsResp.Sessions)
	}
}
