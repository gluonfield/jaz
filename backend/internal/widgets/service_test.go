package widgets_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

func newTestService(t *testing.T) (*widgets.Service, *sqlitestore.Store, loops.Loop) {
	t.Helper()
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loop := loops.Loop{
		ID:         store.NewLoopID(),
		Name:       "PR Tracker",
		Prompt:     "track PRs",
		Schedule:   loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:     loops.StatusActive,
		Runtime:    loops.RuntimeACP,
		MemoryPath: filepath.Join(t.TempDir(), "pr-tracker", "memory.md"),
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.SaveLoop(loop); err != nil {
		t.Fatalf("save loop: %v", err)
	}
	return widgets.NewService(store, nil).WithApps(apps.NewService(store, store, nil, nil, root, root)), store, loop
}

func publish(t *testing.T, service *widgets.Service, loop loops.Loop, runID string, input widgets.PublishInput) widgets.Published {
	t.Helper()
	published, err := service.Publish(context.Background(), loop, runID, "", input)
	if err != nil {
		t.Fatalf("publish %s: %v", runID, err)
	}
	return published
}

func loopApp(t *testing.T, store *sqlitestore.Store, loop loops.Loop) storage.App {
	t.Helper()
	app, err := store.GetAppByLoop(context.Background(), loop.ID)
	if err != nil {
		t.Fatalf("loop app: %v", err)
	}
	return app
}

func makeBoard(t *testing.T, service *widgets.Service, name string) widgets.Board {
	t.Helper()
	board, err := service.CreateBoard(name)
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	return board
}

func TestAssignedLoopPublishesItsAppOntoTheBoard(t *testing.T) {
	service, store, loop := newTestService(t)

	// Unassigned loops cannot publish: assignment is the enablement.
	if _, err := service.Publish(context.Background(), loop, "run-0", "", widgets.PublishInput{HTML: "<p>hi</p>"}); err == nil {
		t.Fatal("expected publish to fail without a board assignment")
	}

	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	items, err := service.ListBoardItems(board.ID)
	if err != nil || len(items) != 1 || items[0].AppID != "" || items[0].CurrentVersion != 0 || items[0].Title != loop.Name {
		t.Fatalf("tile before the first publish = %+v (%v)", items, err)
	}

	first := publish(t, service, loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>", SizeHint: "3x2"})
	if first.App.LoopID != loop.ID || first.App.CurrentVersion != 1 || first.App.Title != loop.Name {
		t.Fatalf("first app = %+v", first.App)
	}
	// Size hint resizes LLM-owned placements.
	placement, _, _ := store.LoadPlacement(board.ID, widget.ID)
	if placement.W != 3 || placement.H != 2 {
		t.Fatalf("placement after hint = %+v", placement)
	}

	// Every later run publishes the next version of the same app.
	second := publish(t, service, loop, "run-2", widgets.PublishInput{HTML: "<p>v2</p>", Title: "Open PRs"})
	if second.App.ID != first.App.ID || second.App.CurrentVersion != 2 {
		t.Fatalf("second publish = %+v, first app %s", second.App, first.App.ID)
	}
	version, err := store.GetAppVersion(context.Background(), second.App.ID, 2)
	if err != nil {
		t.Fatalf("app version: %v", err)
	}
	html, err := os.ReadFile(filepath.Join(version.StorageDir, version.EntryPath))
	if err != nil || string(html) != "<p>v2</p>" {
		t.Fatalf("app content = %q (%v)", html, err)
	}
	items, err = service.ListBoardItems(board.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("board items = %+v (%v)", items, err)
	}
	if item := items[0]; item.WidgetID != widget.ID || item.AppID != first.App.ID || item.LoopID != loop.ID || item.CurrentVersion != 2 || item.Title != "Open PRs" {
		t.Fatalf("tile = %+v", item)
	}
}

func TestAdoptLegacyHTMLTurnsOldTileHTMLIntoTheLoopApp(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(store.RootDir(), "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO widget_versions (widget_id, version, html, created_at_ms) VALUES (?, 3, '<p>stale</p>', 1), (?, 4, '<p>old</p>', 2)`, widget.ID, widget.ID); err != nil {
		t.Fatalf("seed versions: %v", err)
	}
	if _, err := db.Exec(`UPDATE widgets SET current_version = 4 WHERE id = ?`, widget.ID); err != nil {
		t.Fatalf("seed widget: %v", err)
	}

	// A second pass finds nothing left to convert.
	for range 2 {
		if err := service.AdoptLegacyHTML(context.Background()); err != nil {
			t.Fatalf("adopt: %v", err)
		}
	}
	app := loopApp(t, store, loop)
	version, err := store.GetAppVersion(context.Background(), app.ID, 1)
	if err != nil {
		t.Fatalf("app version: %v", err)
	}
	html, err := os.ReadFile(filepath.Join(version.StorageDir, version.EntryPath))
	if err != nil || string(html) != "<p>old</p>" {
		t.Fatalf("app content = %q (%v)", html, err)
	}
	items, err := service.ListBoardItems(board.ID)
	if err != nil || len(items) != 1 || items[0].AppID != app.ID || items[0].CurrentVersion != 1 || items[0].Title != loop.Name {
		t.Fatalf("tile = %+v (%v)", items, err)
	}
}

func TestRequireRunPublishedAcceptsOnlyTheRunThatPublished(t *testing.T) {
	service, store, loop := newTestService(t)
	run := loops.Run{ID: "run-1", LoopID: loop.ID, StartedAt: time.Now()}
	if err := service.RequireRunPublished(loop, run); err != nil {
		t.Fatalf("a loop with no board must not be required to publish: %v", err)
	}
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	publish(t, service, loop, "run-0", widgets.PublishInput{HTML: "<p>old</p>"})

	err = service.RequireRunPublished(loop, run)
	if err == nil || !strings.Contains(err.Error(), "publish_app") {
		t.Fatalf("expected missing-publish error, got %v", err)
	}
	if stored, _ := store.LoadWidget(widget.ID); stored.LastError == "" {
		t.Fatal("the missing publish was not reported on the tile")
	}

	publish(t, service, loop, run.ID, widgets.PublishInput{HTML: "<p>new</p>"})
	if err := service.RequireRunPublished(loop, run); err != nil {
		t.Fatalf("published run rejected: %v", err)
	}
}

func TestAssignMultipleBoardsAndReconcile(t *testing.T) {
	service, store, loop := newTestService(t)
	one := makeBoard(t, service, "One")
	two := makeBoard(t, service, "Two")

	widget, err := service.AssignLoopBoards(loop, []string{one.ID, two.ID})
	if err != nil {
		t.Fatalf("assign two boards: %v", err)
	}
	boards, _ := store.ListBoardsForWidget(widget.ID)
	if len(boards) != 2 {
		t.Fatalf("boards = %v", boards)
	}

	// Reconciling to one board removes the other placement but keeps the widget.
	if _, err := service.AssignLoopBoards(loop, []string{two.ID}); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	boards, _ = store.ListBoardsForWidget(widget.ID)
	if len(boards) != 1 || boards[0] != two.ID {
		t.Fatalf("boards after reconcile = %v", boards)
	}
}

func TestUserPlacementSurvivesPublishAndHint(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	placement, _, _ := store.LoadPlacement(board.ID, widget.ID)
	placement.X, placement.W, placement.PlacedBy = 4, 2, widgets.PlacedByUser
	if err := store.SavePlacement(placement); err != nil {
		t.Fatalf("save placement: %v", err)
	}

	publish(t, service, loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>", SizeHint: "6x4"})
	placement, _, _ = store.LoadPlacement(board.ID, widget.ID)
	if placement.X != 4 || placement.W != 2 || placement.PlacedBy != widgets.PlacedByUser {
		t.Fatalf("publish touched a user placement: %+v", placement)
	}
}

func TestDeleteBoardDisablesButKeepsTheApp(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	first := publish(t, service, loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>"})

	if err := service.DeleteBoard(board.ID); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	// The app and its history survive; the loop is just disabled until reassigned.
	if app := loopApp(t, store, loop); app.ID != first.App.ID || app.CurrentVersion != 1 {
		t.Fatalf("app after board delete = %+v", app)
	}
	if _, err := service.Publish(context.Background(), loop, "run-2", "", widgets.PublishInput{HTML: "<p>v2</p>"}); err == nil {
		t.Fatal("expected publish to fail once no boards remain")
	}

	// Reassigning to a new board re-enables with history intact.
	next := makeBoard(t, service, "Next")
	if _, err := service.AssignLoopBoards(loop, []string{next.ID}); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if again := publish(t, service, loop, "run-3", widgets.PublishInput{HTML: "<p>v2</p>"}); again.App.ID != first.App.ID || again.App.CurrentVersion != 2 {
		t.Fatalf("app after reassign = %+v", again.App)
	}
}

func TestAssignLoopBoardsValidatesBeforeWriting(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")

	// One valid + one unknown board: nothing may be persisted.
	if _, err := service.AssignLoopBoards(loop, []string{board.ID, "board-nope"}); err == nil {
		t.Fatal("expected unknown-board error")
	}
	if _, found, _ := store.LoadWidgetByLoop(loop.ID); found {
		t.Fatal("rejected assignment still created a widget")
	}
	if placements, _ := store.ListPlacements(board.ID); len(placements) != 0 {
		t.Fatalf("rejected assignment still placed tiles: %+v", placements)
	}
}

func TestPatchBoardRejectionLeavesBoardUntouched(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	other := loops.Loop{
		ID: store.NewLoopID(), Name: "Other", Prompt: "p",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:   loops.StatusActive, Runtime: loops.RuntimeACP,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := store.SaveLoop(other); err != nil {
		t.Fatalf("save other loop: %v", err)
	}
	otherWidget, err := service.AssignLoopBoards(other, []string{board.ID})
	if err != nil {
		t.Fatalf("assign other: %v", err)
	}
	first, _, _ := store.LoadPlacement(board.ID, widget.ID)

	// Renaming + overlapping layout in one patch: the rejection must not
	// leave the rename applied.
	name := "Sneaky rename"
	if _, err := service.PatchBoard(board.ID, widgets.UpdateBoard{
		Name: &name,
		Layout: []widgets.LayoutEntry{{
			WidgetID: otherWidget.ID, X: first.X, Y: first.Y, W: first.W, H: first.H,
		}},
	}); err == nil {
		t.Fatal("expected overlap rejection")
	}
	stored, _ := store.LoadBoard(board.ID)
	if stored.Name != "Desk" {
		t.Fatalf("rejected patch still renamed the board: %q", stored.Name)
	}
}

func TestPatchBoardRejectsOverlap(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	other := loops.Loop{
		ID: store.NewLoopID(), Name: "Other", Prompt: "p",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:   loops.StatusActive, Runtime: loops.RuntimeACP,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := store.SaveLoop(other); err != nil {
		t.Fatalf("save other loop: %v", err)
	}
	otherWidget, err := service.AssignLoopBoards(other, []string{board.ID})
	if err != nil {
		t.Fatalf("assign other: %v", err)
	}
	first, _, _ := store.LoadPlacement(board.ID, widget.ID)
	// Moving the second widget onto the first is refused.
	if _, err := service.PatchBoard(board.ID, widgets.UpdateBoard{Layout: []widgets.LayoutEntry{{
		WidgetID: otherWidget.ID, X: first.X, Y: first.Y, W: first.W, H: first.H,
	}}}); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	// Resizing into free space (downward) ignores the tile's own footprint.
	if _, err := service.PatchBoard(board.ID, widgets.UpdateBoard{Layout: []widgets.LayoutEntry{{
		WidgetID: widget.ID, X: first.X, Y: first.Y, W: first.W, H: first.H + 1,
	}}}); err != nil {
		t.Fatalf("self-resize rejected: %v", err)
	}
}

func TestPurgeOrphansFreesCells(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	published := publish(t, service, loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>"})

	// Soft-delete the loop: its tile becomes an orphan squatting on (0,0).
	loop.Status = loops.StatusDeleted
	if err := store.SaveLoop(loop); err != nil {
		t.Fatalf("delete loop: %v", err)
	}
	service.PurgeOrphans()

	if _, found, _ := store.LoadWidgetByLoop(loop.ID); found {
		t.Fatal("orphan widget survived the purge")
	}
	if placements, _ := store.ListPlacements(board.ID); len(placements) != 0 {
		t.Fatalf("orphan placements survived: %+v", placements)
	}
	// The app the loop published stays in the library as an ordinary app.
	if _, err := store.GetAppByLoop(context.Background(), loop.ID); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("deleted loop still owns an app: %v", err)
	}
	if app, err := store.GetApp(context.Background(), published.App.ID); err != nil || app.LoopID != "" {
		t.Fatalf("released app = %+v (%v)", app, err)
	}

	// A fresh loop's widget now lands at the top, not below the phantom.
	fresh := loops.Loop{
		ID: store.NewLoopID(), Name: "Fresh", Prompt: "p",
		Schedule: loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:   loops.StatusActive, Runtime: loops.RuntimeACP,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := store.SaveLoop(fresh); err != nil {
		t.Fatalf("save fresh loop: %v", err)
	}
	freshWidget, err := service.AssignLoopBoards(fresh, []string{board.ID})
	if err != nil {
		t.Fatalf("assign fresh: %v", err)
	}
	placement, _, _ := store.LoadPlacement(board.ID, freshWidget.ID)
	if placement.X != 0 || placement.Y != 0 {
		t.Fatalf("fresh widget did not take the freed top spot: %+v", placement)
	}
}

func TestPublishRejectsFullDocuments(t *testing.T) {
	service, _, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	_, err := service.Publish(context.Background(), loop, "run-1", "", widgets.PublishInput{HTML: "<!doctype html><html><body>hi</body></html>"})
	if err == nil || !strings.Contains(err.Error(), "fragment") {
		t.Fatalf("expected fragment error, got %v", err)
	}
}

func TestPublishLintsAndClearsLayout(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	published := publish(t, service, loop, "run-1", widgets.PublishInput{HTML: `<div style="height:100vh;position:fixed">x</div>`})
	if len(published.Warnings) < 2 {
		t.Fatalf("expected lint warnings for viewport units + fixed position, got %v", published.Warnings)
	}

	// Board telemetry lands on the tile and the prompt surfaces hard
	// rendering failures, but the first version does not yet have a proven
	// useful tile size, so empty space is not treated as a correction signal.
	if err := service.ReportLayout(widget.ID, `{"dead_space_pct":40,"overflow_px":0,"clipped":1,"img_errors":2}`); err != nil {
		t.Fatalf("report layout: %v", err)
	}
	section := service.LoopPromptExtra(loop, loops.Run{})
	if strings.Contains(section, "40% of the tile is empty") {
		t.Fatalf("first version must not optimize for empty space:\n%s", section)
	}
	for _, want := range []string{"Current: version 1", "clip their content", "2 image(s) failed to load"} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt missing %q:\n%s", want, section)
		}
	}

	// …and the next publish clears it.
	publish(t, service, loop, "run-2", widgets.PublishInput{HTML: "<p>v2</p>"})
	if stored, _ := store.LoadWidget(widget.ID); stored.LastLayout != "" {
		t.Fatalf("layout telemetry not cleared on publish: %q", stored.LastLayout)
	}
	if section := service.LoopPromptExtra(loop, loops.Run{}); strings.Contains(section, "Board rendering feedback") {
		t.Fatalf("healthy tile still carries telemetry text:\n%s", section)
	}

	if err := service.ReportLayout(widget.ID, `{"dead_space_pct":40,"overflow_px":0,"clipped":0,"img_errors":0}`); err != nil {
		t.Fatalf("report layout after second publish: %v", err)
	}
	if section := service.LoopPromptExtra(loop, loops.Run{}); !strings.Contains(section, "40% of the tile is empty") {
		t.Fatalf("established versions should surface empty-space telemetry:\n%s", section)
	}
}

func TestReportErrorClearedByNextPublish(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	publish(t, service, loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>"})
	if err := service.ReportError(widget.ID, "ReferenceError: foo is not defined"); err != nil {
		t.Fatalf("report error: %v", err)
	}
	stored, _ := store.LoadWidget(widget.ID)
	if stored.LastError == "" {
		t.Fatal("expected last error to be stored")
	}
	publish(t, service, loop, "run-2", widgets.PublishInput{HTML: "<p>v2</p>"})
	stored, _ = store.LoadWidget(widget.ID)
	if stored.LastError != "" {
		t.Fatalf("expected last error cleared, got %q", stored.LastError)
	}
}

func TestPromptSectionMentionsFileAndErrors(t *testing.T) {
	_, _, loop := newTestService(t)
	section := widgets.PromptSection(loop, widgets.Widget{Title: "Open PRs", LastError: "boom"}, 3)
	for _, want := range []string{
		"## Board App Runtime",
		widgets.WidgetFilePath(loop),
		"publish_app",
		"A list or feed is fine",
		"boom",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt section missing %q:\n%s", want, section)
		}
	}
	if strings.Contains(section, "Visual Creation Suite") {
		t.Fatal("design system leaked into the prompt; agents should fetch it with artifact_guide")
	}
	for _, reject := range []string{
		"visualize",
		"visualise:",
		"show_artifact",
		"if it is available",
		"If no such tool",
		"extension method",
		"If no publish mechanism",
		"fallback",
		"Tile quality floor",
		"compact rows",
	} {
		if strings.Contains(section, reject) {
			t.Fatalf("prompt must expose only the board app contract; found %q:\n%s", reject, section)
		}
	}
	if strings.Contains(section, "AGENTS.md") || strings.Contains(section, "design guide next to it") {
		t.Fatalf("prompt must not point at a generated guide file:\n%s", section)
	}
	if !strings.Contains(section, "create or overwrite it this run") {
		t.Fatalf("prompt must use the invariant file contract:\n%s", section)
	}
}
