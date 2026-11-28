package boards_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/apps"
	boardscore "github.com/wins/jaz/backend/internal/boards"
	boardsapi "github.com/wins/jaz/backend/internal/httpapi/boards"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type cloud struct{ proxied bool }

func (c *cloud) ProxyActiveWorkspace(w http.ResponseWriter, _ *http.Request) bool {
	if c.proxied {
		w.WriteHeader(http.StatusTeapot)
	}
	return c.proxied
}

func TestBoardLayoutAPI(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	appService := apps.NewService(store, store, nil, nil, root, root)
	local := func() string { return storage.LocalWorkspaceID }
	service := boardscore.NewService(store, local, log.New(io.Discard)).WithApps(appService)
	now := time.Now().UTC()
	loop := loops.Loop{
		ID:        store.NewLoopID(),
		Name:      "Inbox",
		Prompt:    "p",
		Schedule:  loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:    loops.StatusActive,
		Runtime:   loops.RuntimeACP,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.SaveLoop(loop); err != nil {
		t.Fatal(err)
	}
	board, err := appService.CreateBoard(ctx, "Desk")
	if err != nil {
		t.Fatal(err)
	}
	notes, err := appService.PublishLoopApp(ctx, "", "Notes", "<p>notes</p>", "")
	if err != nil {
		t.Fatal(err)
	}

	proxy := &cloud{}
	mux := http.NewServeMux()
	for _, route := range app.BoardRoutes(boardsapi.NewHandler(service, loops.NewService(store, nil, nil), proxy)) {
		mux.Handle(route.Pattern, route.Handler)
	}
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(method, path, strings.NewReader(body)))
		if res.Code != want {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	view := func() boardscore.View {
		t.Helper()
		var out boardscore.View
		if err := json.Unmarshal(call(http.MethodGet, "/v1/apps/"+board.ID+"/board", "", http.StatusOK), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	base := "/v1/apps/" + board.ID + "/board"

	if empty := view(); empty.GridCols != boardscore.GridCols || empty.RowHeight != boardscore.RowHeight || empty.FontScale != 1 || len(empty.Tiles) != 0 {
		t.Fatalf("empty board = %+v", empty)
	}
	call(http.MethodPost, base+"/tiles", `{"app_ids":["`+notes.ID+`"],"loop_ids":["`+loop.ID+`"]}`, http.StatusOK)
	tiles := view().Tiles
	if len(tiles) != 2 || tiles[0].App.ID != notes.ID || tiles[0].Loop != nil || tiles[1].Loop == nil || tiles[1].Loop.ID != loop.ID || tiles[1].Loop.Name != "Inbox" || tiles[1].App.CurrentVersion != 0 {
		t.Fatalf("tiles = %+v", tiles)
	}
	loopAppID := tiles[1].App.ID

	// The user's layout sticks; an overlapping one is refused whole.
	call(http.MethodPatch, base, `{"font_scale":1.4,"layout":[{"app_id":"`+loopAppID+`","x":4,"y":0,"w":2,"h":3}]}`, http.StatusOK)
	call(http.MethodPatch, base, `{"layout":[{"app_id":"`+loopAppID+`","x":0,"y":0,"w":2,"h":2}]}`, http.StatusBadRequest)
	moved := view()
	if moved.FontScale != 1.4 || moved.Tiles[1].X != 4 || moved.Tiles[1].W != 2 || moved.Tiles[1].H != 3 {
		t.Fatalf("board after layout = %+v", moved)
	}

	call(http.MethodPost, "/v1/apps/"+loopAppID+"/tile-feedback", `{"error":"TypeError: x is undefined","layout":{"clipped":2}}`, http.StatusOK)
	if tile := view().Tiles[1]; tile.LastError != "TypeError: x is undefined" {
		t.Fatalf("tile after feedback = %+v", tile)
	}
	if loopApp, err := store.GetLoopApp(ctx, loop.ID); err != nil || loopApp.LastLayout != `{"clipped":2}` {
		t.Fatalf("layout feedback = %+v (%v)", loopApp, err)
	}

	call(http.MethodDelete, base+"/tiles/"+notes.ID, "", http.StatusOK)
	if tiles := view().Tiles; len(tiles) != 1 || tiles[0].App.ID != loopAppID {
		t.Fatalf("tiles after remove = %+v", tiles)
	}

	call(http.MethodPost, base+"/tiles", `{"loop_ids":["nope"]}`, http.StatusBadRequest)
	call(http.MethodPost, base+"/tiles", `{}`, http.StatusBadRequest)
	call(http.MethodPost, base+"/tiles", `{"app_ids":["`+board.ID+`"]}`, http.StatusBadRequest)
	call(http.MethodGet, "/v1/apps/"+notes.ID+"/board", "", http.StatusNotFound)
	call(http.MethodPatch, "/v1/apps/missing/board", `{}`, http.StatusNotFound)

	// A Cloud workspace's boards live in Jaz Cloud.
	proxy.proxied = true
	call(http.MethodGet, base, "", http.StatusTeapot)
	call(http.MethodPost, "/v1/apps/"+loopAppID+"/tile-feedback", `{}`, http.StatusTeapot)
}
