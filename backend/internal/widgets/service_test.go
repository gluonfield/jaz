package widgets_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/widgets"
)

func newTestService(t *testing.T) (*widgets.Service, *sqlitestore.Store, loops.Loop) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
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
	return widgets.NewService(store, nil), store, loop
}

func makeBoard(t *testing.T, service *widgets.Service, name string) widgets.Board {
	t.Helper()
	board, err := service.CreateBoard(name)
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	return board
}

func TestAssignmentEnablesWidget(t *testing.T) {
	service, store, loop := newTestService(t)

	// Unassigned loops cannot publish: assignment is the enablement.
	if _, _, err := service.Publish(loop, "run-0", widgets.PublishInput{HTML: "<p>hi</p>"}); err == nil {
		t.Fatal("expected publish to fail without a board assignment")
	}

	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if widget.CurrentVersion != 0 || widget.Title != loop.Name {
		t.Fatalf("eager widget = %+v", widget)
	}
	if _, found, _ := store.LoadPlacement(board.ID, widget.ID); !found {
		t.Fatal("expected placement on assignment")
	}

	published, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>", SizeHint: "3x2"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.CurrentVersion != 1 {
		t.Fatalf("version = %d", published.CurrentVersion)
	}
	// Size hint resizes LLM-owned placements.
	placement, _, _ := store.LoadPlacement(board.ID, widget.ID)
	if placement.W != 3 || placement.H != 2 {
		t.Fatalf("placement after hint = %+v", placement)
	}
}

func TestPublishKeepsCurrentAndFiveOldVersions(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	for version := 1; version <= 8; version++ {
		if _, _, err := service.Publish(loop, fmt.Sprintf("run-%d", version), widgets.PublishInput{HTML: fmt.Sprintf("<p>v%d</p>", version)}); err != nil {
			t.Fatalf("publish v%d: %v", version, err)
		}
	}
	stored, _ := store.LoadWidget(widget.ID)
	if stored.CurrentVersion != 8 {
		t.Fatalf("current version = %d", stored.CurrentVersion)
	}
	if _, err := store.LoadWidgetVersion(widget.ID, 2); err == nil {
		t.Fatal("expected versions older than the five-version history to be pruned")
	}
	for version := 3; version <= 8; version++ {
		if _, err := store.LoadWidgetVersion(widget.ID, version); err != nil {
			t.Fatalf("version %d was pruned: %v", version, err)
		}
	}
}

func TestRunPublishStateRequiresCurrentVersionFromRun(t *testing.T) {
	service, _, loop := newTestService(t)
	state, err := service.RunPublishState(loop.ID, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled || state.Published {
		t.Fatalf("unassigned state = %+v", state)
	}
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	state, err = service.RunPublishState(loop.ID, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || state.Published {
		t.Fatalf("unpublished state = %+v", state)
	}
	if _, _, err := service.Publish(loop, "run-0", widgets.PublishInput{HTML: "<p>old</p>"}); err != nil {
		t.Fatalf("publish old: %v", err)
	}
	state, err = service.RunPublishState(loop.ID, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || state.Published {
		t.Fatalf("stale state = %+v", state)
	}
	if _, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: "<p>new</p>"}); err != nil {
		t.Fatalf("publish current: %v", err)
	}
	state, err = service.RunPublishState(loop.ID, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || !state.Published {
		t.Fatalf("current state = %+v", state)
	}
}

func TestEnsureRunPublishedRejectsStaleWidgetFile(t *testing.T) {
	service, _, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	path := widgets.WidgetFilePath(loop)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<p>old</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := loops.Run{ID: "run-1", LoopID: loop.ID, StartedAt: time.Now().Add(time.Hour)}
	err := service.EnsureRunPublished(loop, run)
	if err == nil || !strings.Contains(err.Error(), "not updated during this run") {
		t.Fatalf("expected stale-file error, got %v", err)
	}
	state, stateErr := service.RunPublishState(loop.ID, run.ID)
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if state.Published || state.Widget.LastError == "" {
		t.Fatalf("state = %+v", state)
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

	if _, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>", SizeHint: "6x4"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	placement, _, _ = store.LoadPlacement(board.ID, widget.ID)
	if placement.X != 4 || placement.W != 2 || placement.PlacedBy != widgets.PlacedByUser {
		t.Fatalf("publish touched a user placement: %+v", placement)
	}
}

func TestDeleteBoardDisablesButKeepsWidget(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if _, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if err := service.DeleteBoard(board.ID); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	// Widget and history survive; the loop is just disabled until reassigned.
	stored, found, err := store.LoadWidgetByLoop(loop.ID)
	if err != nil || !found || stored.CurrentVersion != 1 {
		t.Fatalf("widget after board delete = %#v (%v)", stored, err)
	}
	if _, err := store.LoadWidgetVersion(widget.ID, 1); err != nil {
		t.Fatalf("version lost on board delete: %v", err)
	}
	if _, _, err := service.Publish(loop, "run-2", widgets.PublishInput{HTML: "<p>v2</p>"}); err == nil {
		t.Fatal("expected publish to fail once no boards remain")
	}

	// Reassigning to a new board re-enables with history intact.
	next := makeBoard(t, service, "Next")
	if _, err := service.AssignLoopBoards(loop, []string{next.ID}); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	published, _, err := service.Publish(loop, "run-3", widgets.PublishInput{HTML: "<p>v2</p>"})
	if err != nil {
		t.Fatalf("publish after reassign: %v", err)
	}
	if published.CurrentVersion != 2 {
		t.Fatalf("version after reassign = %d", published.CurrentVersion)
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
	widget, err := service.AssignLoopBoards(loop, []string{board.ID})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Soft-delete the loop: its widget becomes an orphan squatting on (0,0).
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
	if _, err := store.LoadWidgetVersion(widget.ID, 1); err == nil {
		t.Fatal("orphan widget versions survived")
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
	_, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: "<!doctype html><html><body>hi</body></html>"})
	if err == nil || !strings.Contains(err.Error(), "fragment") {
		t.Fatalf("expected fragment error, got %v", err)
	}
}

func TestPublishLintsAndClearsLayout(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	widget, warnings, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: `<div style="height:100vh;position:fixed">x</div>`})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected lint warnings for viewport units + fixed position, got %v", warnings)
	}

	// Board telemetry lands on the widget and the prompt surfaces it…
	if err := service.ReportLayout(widget.ID, `{"dead_space_pct":40,"overflow_px":0,"clipped":1,"img_errors":2}`); err != nil {
		t.Fatalf("report layout: %v", err)
	}
	stored, _ := store.LoadWidget(widget.ID)
	section := widgets.PromptSection(loop, &stored)
	for _, want := range []string{"40% of the tile is empty", "clip their content", "2 image(s) failed to load"} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt missing telemetry %q:\n%s", want, section)
		}
	}

	// …and the next publish clears it.
	if _, _, err := service.Publish(loop, "run-2", widgets.PublishInput{HTML: "<p>v2</p>"}); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	stored, _ = store.LoadWidget(widget.ID)
	if stored.LastLayout != "" {
		t.Fatalf("layout telemetry not cleared on publish: %q", stored.LastLayout)
	}
	if section := widgets.PromptSection(loop, &stored); strings.Contains(section, "telemetry") {
		t.Fatalf("healthy widget still carries telemetry text:\n%s", section)
	}
}

func TestReportErrorClearedByNextPublish(t *testing.T) {
	service, store, loop := newTestService(t)
	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	widget, _, err := service.Publish(loop, "run-1", widgets.PublishInput{HTML: "<p>v1</p>"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := service.ReportError(widget.ID, "ReferenceError: foo is not defined"); err != nil {
		t.Fatalf("report error: %v", err)
	}
	stored, _ := store.LoadWidget(widget.ID)
	if stored.LastError == "" {
		t.Fatal("expected last error to be stored")
	}
	if _, _, err := service.Publish(loop, "run-2", widgets.PublishInput{HTML: "<p>v2</p>"}); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	stored, _ = store.LoadWidget(widget.ID)
	if stored.LastError != "" {
		t.Fatalf("expected last error cleared, got %q", stored.LastError)
	}
}

func TestPromptSectionMentionsFileAndErrors(t *testing.T) {
	_, _, loop := newTestService(t)
	section := widgets.PromptSection(loop, &widgets.Widget{CurrentVersion: 3, Title: "Open PRs", LastError: "boom"})
	for _, want := range []string{
		widgets.WidgetFilePath(loop),
		"visualise:read_me",
		"jaztools",
		"Tile quality floor",
		"visualise:publish_widget",
		"boom",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt section missing %q:\n%s", want, section)
		}
	}
	if strings.Contains(section, "Visual Creation Suite") {
		t.Fatal("design system leaked into the prompt; agents should fetch it with visualise:read_me")
	}
	for _, reject := range []string{
		"visualize",
		"visualise_read_me",
		"visualise:show_widget",
		"`publish_widget`",
		"if it is available",
		"If no such tool",
		"extension method",
		"If no publish mechanism",
		"fallback",
	} {
		if strings.Contains(section, reject) {
			t.Fatalf("prompt must expose one required visualise:* widget tool contract; found %q:\n%s", reject, section)
		}
	}
	if strings.Contains(section, "AGENTS.md") || strings.Contains(section, "design guide next to it") {
		t.Fatalf("prompt must not point at a generated guide file:\n%s", section)
	}
	if !strings.Contains(section, "create or overwrite it this run") {
		t.Fatalf("prompt must use the invariant file contract:\n%s", section)
	}
}

// Widget documents are now built client-side by the shared artifact renderer;
// the server returns the raw fragment (see TestWidgetContentServesRawFragment
// in the server package).
