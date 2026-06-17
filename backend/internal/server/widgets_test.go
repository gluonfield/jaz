package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

func newWidgetTestServer(t *testing.T) (*Server, *sqlitestore.Store, *widgets.Service) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := widgets.NewService(store, nil)
	executor := &fakeLoopExecutor{started: make(chan loops.Run, 1)}
	srv := &Server{Store: store, Widgets: service, Loops: newLoopServiceForTest(store, executor)}
	return srv, store, service
}

func saveTestLoop(t *testing.T, store *sqlitestore.Store, name string) loops.Loop {
	t.Helper()
	loop := loops.Loop{
		ID:        store.NewLoopID(),
		Name:      name,
		Prompt:    "do the thing",
		Schedule:  loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:    loops.StatusActive,
		Runtime:   loops.RuntimeACP,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.SaveLoop(loop); err != nil {
		t.Fatal(err)
	}
	return loop
}

func TestBoardsAPIAssignmentFlow(t *testing.T) {
	srv, store, service := newWidgetTestServer(t)
	loop := saveTestLoop(t, store, "Inbox")

	// No bootstrap: a fresh install has zero boards until onboarding creates one.
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/boards", nil))
	var listed struct {
		Boards []widgets.Board `json:"boards"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Boards) != 0 {
		t.Fatalf("expected no boards on fresh install, got %#v", listed.Boards)
	}

	create := httptest.NewRequest(http.MethodPost, "/v1/boards", strings.NewReader(`{"name":"Desk"}`))
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, create)
	var board widgets.Board
	if err := json.Unmarshal(res.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}

	// Onboarding's final step: assign existing loops to the new board.
	assign := httptest.NewRequest(http.MethodPost, "/v1/boards/"+board.ID+"/loops",
		strings.NewReader(`{"loop_ids":["`+loop.ID+`"]}`))
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, assign)
	if res.Code != http.StatusOK {
		t.Fatalf("assign = %d: %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/boards/"+board.ID, nil))
	var detail struct {
		Items []widgets.BoardItem `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Items) != 1 || detail.Items[0].LoopID != loop.ID || detail.Items[0].CurrentVersion != 0 {
		t.Fatalf("board items = %#v", detail.Items)
	}
	widgetID := detail.Items[0].WidgetID

	// Loop detail exposes the assignment.
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/loops/"+loop.ID, nil))
	var loopDetail struct {
		BoardIDs []string `json:"board_ids"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &loopDetail); err != nil {
		t.Fatal(err)
	}
	if len(loopDetail.BoardIDs) != 1 || loopDetail.BoardIDs[0] != board.ID {
		t.Fatalf("loop board_ids = %v", loopDetail.BoardIDs)
	}

	// Patching board_ids to empty unassigns (disables the widget).
	patch := httptest.NewRequest(http.MethodPatch, "/v1/loops/"+loop.ID, strings.NewReader(`{"board_ids":[]}`))
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, patch)
	if res.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", res.Code, res.Body.String())
	}
	boards, err := store.ListBoardsForWidget(widgetID)
	if err != nil || len(boards) != 0 {
		t.Fatalf("boards after unassign = %v (%v)", boards, err)
	}

	// User layout updates stick and flip placed_by.
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatal(err)
	}
	layout := httptest.NewRequest(http.MethodPatch, "/v1/boards/"+board.ID,
		strings.NewReader(`{"layout":[{"widget_id":"`+widgetID+`","x":3,"y":1,"w":3,"h":2}]}`))
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, layout)
	if res.Code != http.StatusOK {
		t.Fatalf("patch layout = %d: %s", res.Code, res.Body.String())
	}
	placement, found, err := store.LoadPlacement(board.ID, widgetID)
	if err != nil || !found {
		t.Fatalf("placement missing: %v", err)
	}
	if placement.X != 3 || placement.W != 3 || placement.PlacedBy != widgets.PlacedByUser {
		t.Fatalf("placement = %#v", placement)
	}

	// Deleting the board disables the widget but keeps it.
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/v1/boards/"+board.ID, nil))
	if res.Code != http.StatusOK {
		t.Fatalf("delete board = %d: %s", res.Code, res.Body.String())
	}
	if _, found, _ := store.LoadWidgetByLoop(loop.ID); !found {
		t.Fatal("widget should survive board deletion")
	}
}

func TestWidgetContentAndErrorsAPI(t *testing.T) {
	srv, store, service := newWidgetTestServer(t)
	loop := saveTestLoop(t, store, "Inbox")
	board, err := service.CreateBoard("Desk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatal(err)
	}
	widget, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: `<p id="hello">hi</p>`})
	if err != nil {
		t.Fatal(err)
	}

	// The content endpoint serves the raw fragment; the board wraps it in the
	// shared artifact document client-side, so no host chrome leaks here.
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/widgets/"+widget.ID+"/content", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("content = %d: %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content type = %q", got)
	}
	body := res.Body.String()
	if strings.TrimSpace(body) != `<p id="hello">hi</p>` {
		t.Fatalf("content should be the raw fragment, got: %q", body)
	}

	report := httptest.NewRequest(http.MethodPost, "/v1/widgets/"+widget.ID+"/errors",
		strings.NewReader(`{"message":"TypeError: x is undefined"}`))
	res = httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, report)
	if res.Code != http.StatusOK {
		t.Fatalf("report error = %d: %s", res.Code, res.Body.String())
	}
	stored, err := store.LoadWidget(widget.ID)
	if err != nil || stored.LastError == "" {
		t.Fatalf("widget error not stored: %v %#v", err, stored)
	}
}
