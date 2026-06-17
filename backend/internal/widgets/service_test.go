package widgets_test

import (
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

func TestNormalizeBoardLayoutsUnburiesTiles(t *testing.T) {
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
	// Bury the first widget under the second (legacy overlapping data).
	placement, _, _ := store.LoadPlacement(board.ID, widget.ID)
	buried, _, _ := store.LoadPlacement(board.ID, otherWidget.ID)
	buried.X, buried.Y, buried.W, buried.H = placement.X, placement.Y, placement.W, placement.H
	buried.UpdatedAt = placement.UpdatedAt.Add(-time.Hour) // older — loses the spot
	if err := store.SavePlacement(buried); err != nil {
		t.Fatalf("bury: %v", err)
	}

	service.NormalizeBoardLayouts()

	a, _, _ := store.LoadPlacement(board.ID, widget.ID)
	b, _, _ := store.LoadPlacement(board.ID, otherWidget.ID)
	if a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H {
		t.Fatalf("placements still overlap: %+v vs %+v", a, b)
	}
	if a.X != placement.X || a.Y != placement.Y {
		t.Fatalf("recently touched tile lost its spot: %+v", a)
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

func TestMaybeAutoPublishRequiresAssignment(t *testing.T) {
	service, store, loop := newTestService(t)
	path := widgets.WidgetFilePath(loop)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("<p>from disk</p>"), 0o644); err != nil {
		t.Fatalf("write widget file: %v", err)
	}

	// No assignment: file changes are ignored.
	service.MaybeAutoPublish(loop, "run-0")
	if _, found, _ := store.LoadWidgetByLoop(loop.ID); found {
		t.Fatal("auto-publish created a widget without assignment")
	}

	board := makeBoard(t, service, "Desk")
	if _, err := service.AssignLoopBoards(loop, []string{board.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	service.MaybeAutoPublish(loop, "run-1")
	widget, found, _ := store.LoadWidgetByLoop(loop.ID)
	if !found || widget.CurrentVersion != 1 {
		t.Fatalf("auto-publish after assignment = %#v", widget)
	}
	// Unchanged file does not produce a new version.
	service.MaybeAutoPublish(loop, "run-2")
	widget, _, _ = store.LoadWidgetByLoop(loop.ID)
	if widget.CurrentVersion != 1 {
		t.Fatalf("auto-publish republished unchanged content: version %d", widget.CurrentVersion)
	}
}

func TestPromptSectionMentionsFileAndErrors(t *testing.T) {
	_, _, loop := newTestService(t)
	section := widgets.PromptSection(loop, &widgets.Widget{CurrentVersion: 3, Title: "Open PRs", LastError: "boom"})
	for _, want := range []string{
		widgets.WidgetFilePath(loop),
		"visualize:read_me",
		"jaztools",
		"agent-side MCP-mapped read_me",
		"Tile quality floor",
		"publish_widget",
		"Do not call `visualize:show_widget`",
		"boom",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt section missing %q:\n%s", want, section)
		}
	}
	if strings.Contains(section, "Visual Creation Suite") {
		t.Fatal("design system leaked into the prompt; agents should fetch it with visualize:read_me")
	}
	if strings.Contains(section, "AGENTS.md") || strings.Contains(section, "design guide next to it") {
		t.Fatalf("prompt must not point at a generated guide file:\n%s", section)
	}

	// A missing widget file is announced up front, not discovered via a failed read…
	if !strings.Contains(section, "does not exist yet") {
		t.Fatalf("prompt must announce a missing widget file:\n%s", section)
	}
	// …and once it exists the agent is told to iterate on it.
	path := widgets.WidgetFilePath(loop)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("<p>v3</p>"), 0o644); err != nil {
		t.Fatalf("write widget file: %v", err)
	}
	section = widgets.PromptSection(loop, &widgets.Widget{CurrentVersion: 3, Title: "Open PRs"})
	if !strings.Contains(section, "iterate on it") || strings.Contains(section, "does not exist yet") {
		t.Fatalf("prompt must tell the agent to iterate on the existing file:\n%s", section)
	}
}

// Widget documents are now built client-side by the shared artifact renderer;
// the server returns the raw fragment (see TestWidgetContentServesRawFragment
// in the server package).
