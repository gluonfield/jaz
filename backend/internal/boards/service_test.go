package boards_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/apps"
	"github.com/wins/jaz/backend/internal/boards"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fixture struct {
	service   *boards.Service
	apps      *apps.Service
	store     *sqlitestore.Store
	loop      loops.Loop
	workspace string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	f := &fixture{store: store, workspace: storage.LocalWorkspaceID}
	f.apps = apps.NewService(store, store, nil, nil, root, root)
	f.service = boards.NewService(store, func() string { return f.workspace }, log.New(io.Discard)).WithApps(f.apps)
	f.loop = f.saveLoop(t, "PR Tracker")
	return f
}

func (f *fixture) saveLoop(t *testing.T, name string) loops.Loop {
	t.Helper()
	loop := loops.Loop{
		ID:         f.store.NewLoopID(),
		Name:       name,
		Prompt:     "track PRs",
		Schedule:   loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
		Status:     loops.StatusActive,
		Runtime:    loops.RuntimeACP,
		MemoryPath: filepath.Join(t.TempDir(), name, "memory.md"),
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := f.store.SaveLoop(loop); err != nil {
		t.Fatalf("save loop: %v", err)
	}
	return loop
}

func (f *fixture) board(t *testing.T, title string) string {
	t.Helper()
	board, err := f.apps.CreateBoard(context.Background(), title)
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	return board.ID
}

func (f *fixture) assign(t *testing.T, loop loops.Loop, boardIDs ...string) {
	t.Helper()
	if err := f.service.AssignLoopBoards(context.Background(), loop, boardIDs); err != nil {
		t.Fatalf("assign: %v", err)
	}
}

func (f *fixture) publish(t *testing.T, runID string, input boards.PublishInput) boards.Published {
	t.Helper()
	published, err := f.service.Publish(context.Background(), f.loop, runID, "", input)
	if err != nil {
		t.Fatalf("publish %s: %v", runID, err)
	}
	return published
}

func (f *fixture) tiles(t *testing.T, boardID string) []storage.BoardTile {
	t.Helper()
	view, err := f.service.View(context.Background(), boardID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	return view.Tiles
}

func (f *fixture) loopApp(t *testing.T, loop loops.Loop) storage.LoopApp {
	t.Helper()
	loopApp, err := f.store.GetLoopApp(context.Background(), loop.ID)
	if err != nil {
		t.Fatalf("loop app: %v", err)
	}
	return loopApp
}

func TestAssignedLoopPublishesItsAppOntoTheBoard(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// A loop publishes an app only while it is on a board.
	if _, err := f.service.Publish(ctx, f.loop, "run-0", "", boards.PublishInput{HTML: "<p>hi</p>"}); err == nil {
		t.Fatal("expected publish to fail without a board assignment")
	}

	boardID := f.board(t, "Desk")
	f.assign(t, f.loop, boardID)
	tiles := f.tiles(t, boardID)
	if len(tiles) != 1 || tiles[0].App.CurrentVersion != 0 || tiles[0].App.Title != f.loop.Name || tiles[0].Loop == nil || tiles[0].Loop.ID != f.loop.ID {
		t.Fatalf("tile before the first publish = %+v", tiles)
	}
	// The waiting app is a tile, not yet a library item.
	if library, err := f.apps.ListApps(ctx, storage.AppListFilter{Kind: storage.AppKindArtifact}); err != nil || len(library) != 0 {
		t.Fatalf("library before the first publish = %+v (%v)", library, err)
	}

	first := f.publish(t, "run-1", boards.PublishInput{HTML: "<p>v1</p>", SizeHint: "3x2"})
	if first.App.ID != tiles[0].App.ID || first.App.LoopID != f.loop.ID || first.App.CurrentVersion != 1 || first.App.Title != f.loop.Name {
		t.Fatalf("first app = %+v", first.App)
	}
	// The size hint resizes tiles the user has not placed.
	if tile := f.tiles(t, boardID)[0]; tile.W != 3 || tile.H != 2 {
		t.Fatalf("tile after hint = %+v", tile)
	}

	// Every later run publishes the next version of the same app.
	second := f.publish(t, "run-2", boards.PublishInput{HTML: "<p>v2</p>", Title: "Open PRs"})
	if second.App.ID != first.App.ID || second.App.CurrentVersion != 2 {
		t.Fatalf("second publish = %+v, first app %s", second.App, first.App.ID)
	}
	version, err := f.store.GetAppVersion(ctx, second.App.ID, 2)
	if err != nil {
		t.Fatalf("app version: %v", err)
	}
	html, err := os.ReadFile(filepath.Join(version.StorageDir, version.EntryPath))
	if err != nil || string(html) != "<p>v2</p>" {
		t.Fatalf("app content = %q (%v)", html, err)
	}
	// A run that names no title keeps the one the app has.
	third := f.publish(t, "run-3", boards.PublishInput{HTML: "<p>v3</p>"})
	if tile := f.tiles(t, boardID)[0]; tile.App.ID != first.App.ID || tile.App.CurrentVersion != 3 || tile.App.Title != "Open PRs" || third.App.Title != "Open PRs" {
		t.Fatalf("tile = %+v", tile)
	}
}

func TestAdoptLegacyHTMLTurnsOldTileHTMLIntoTheLoopApp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")
	f.assign(t, f.loop, boardID)
	appID := f.loopApp(t, f.loop).AppID
	db, err := sql.Open("sqlite", filepath.Join(f.store.RootDir(), "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO legacy_widget_html (app_id, html) VALUES (?, '<p>old</p>')`, appID); err != nil {
		t.Fatalf("seed legacy html: %v", err)
	}

	// A second pass finds nothing left to convert.
	for range 2 {
		if err := f.service.AdoptLegacyHTML(ctx); err != nil {
			t.Fatalf("adopt: %v", err)
		}
	}
	version, err := f.store.GetAppVersion(ctx, appID, 1)
	if err != nil {
		t.Fatalf("app version: %v", err)
	}
	html, err := os.ReadFile(filepath.Join(version.StorageDir, version.EntryPath))
	if err != nil || string(html) != "<p>old</p>" {
		t.Fatalf("app content = %q (%v)", html, err)
	}
	if tiles := f.tiles(t, boardID); len(tiles) != 1 || tiles[0].App.ID != appID || tiles[0].App.CurrentVersion != 1 || tiles[0].App.Title != f.loop.Name {
		t.Fatalf("tile = %+v", tiles)
	}
}

func TestRequireRunPublishedAcceptsOnlyTheRunThatPublished(t *testing.T) {
	f := newFixture(t)
	run := loops.Run{ID: "run-1", LoopID: f.loop.ID, StartedAt: time.Now()}
	if err := f.service.RequireRunPublished(f.loop, run); err != nil {
		t.Fatalf("a loop with no board must not be required to publish: %v", err)
	}
	f.assign(t, f.loop, f.board(t, "Desk"))
	f.publish(t, "run-0", boards.PublishInput{HTML: "<p>old</p>"})

	err := f.service.RequireRunPublished(f.loop, run)
	if err == nil || !strings.Contains(err.Error(), "publish_app") {
		t.Fatalf("expected missing-publish error, got %v", err)
	}
	if f.loopApp(t, f.loop).LastError == "" {
		t.Fatal("the missing publish was not reported on the tile")
	}

	f.publish(t, run.ID, boards.PublishInput{HTML: "<p>new</p>"})
	if err := f.service.RequireRunPublished(f.loop, run); err != nil {
		t.Fatalf("published run rejected: %v", err)
	}
}

func TestAssignLoopBoardsReconcilesToExactlyTheGivenBoards(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	one := f.board(t, "One")
	two := f.board(t, "Two")

	f.assign(t, f.loop, one, two)
	if _, assigned, err := f.service.StateForLoop(ctx, f.loop.ID); err != nil || len(assigned) != 2 {
		t.Fatalf("boards = %v (%v)", assigned, err)
	}
	appID := f.loopApp(t, f.loop).AppID

	f.assign(t, f.loop, two)
	loopApp, assigned, err := f.service.StateForLoop(ctx, f.loop.ID)
	if err != nil || len(assigned) != 1 || assigned[0] != two || loopApp.AppID != appID {
		t.Fatalf("after reconcile: app %s on %v (%v)", loopApp.AppID, assigned, err)
	}
}

func TestAssignLoopBoardsValidatesBeforeWriting(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")

	if err := f.service.AssignLoopBoards(ctx, f.loop, []string{boardID, "board-nope"}); err == nil {
		t.Fatal("expected unknown-board error")
	}
	if _, err := f.store.GetLoopApp(ctx, f.loop.ID); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("rejected assignment still created an app: %v", err)
	}
	if tiles := f.tiles(t, boardID); len(tiles) != 0 {
		t.Fatalf("rejected assignment still placed tiles: %+v", tiles)
	}
	// An ordinary app is not a board a loop can be assigned to.
	f.assign(t, f.loop, boardID)
	if err := f.service.AssignLoopBoards(ctx, f.loop, []string{f.loopApp(t, f.loop).AppID}); err == nil {
		t.Fatal("an app that is not a board was accepted as one")
	}
}

func TestUserLayoutSurvivesPublishAndHint(t *testing.T) {
	f := newFixture(t)
	boardID := f.board(t, "Desk")
	f.assign(t, f.loop, boardID)
	appID := f.loopApp(t, f.loop).AppID
	if err := f.service.Patch(context.Background(), boardID, boards.Update{Layout: []boards.LayoutEntry{{AppID: appID, X: 4, Y: 0, W: 2, H: 2}}}); err != nil {
		t.Fatalf("patch: %v", err)
	}

	f.publish(t, "run-1", boards.PublishInput{HTML: "<p>v1</p>", SizeHint: "6x4"})
	if tile := f.tiles(t, boardID)[0]; tile.X != 4 || tile.W != 2 || tile.H != 2 {
		t.Fatalf("publish resized a tile the user placed: %+v", tile)
	}
}

func TestPatchRejectsOverlapAndWritesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")
	other := f.saveLoop(t, "Other")
	f.assign(t, f.loop, boardID)
	f.assign(t, other, boardID)
	first := f.tiles(t, boardID)[0]
	otherID := f.loopApp(t, other).AppID

	scale := 1.5
	err := f.service.Patch(ctx, boardID, boards.Update{
		FontScale: &scale,
		Layout:    []boards.LayoutEntry{{AppID: otherID, X: first.X, Y: first.Y, W: first.W, H: first.H}},
	})
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	if view, _ := f.service.View(ctx, boardID); view.FontScale != 1 {
		t.Fatalf("rejected patch still zoomed the board: %v", view.FontScale)
	}
	// Growing a tile into free space ignores its own footprint.
	if err := f.service.Patch(ctx, boardID, boards.Update{Layout: []boards.LayoutEntry{{AppID: first.App.ID, X: first.X, Y: first.Y + 2, W: first.W, H: first.H + 1}}}); err != nil {
		t.Fatalf("self-resize rejected: %v", err)
	}
	if err := f.service.Patch(ctx, boardID, boards.Update{Layout: []boards.LayoutEntry{{AppID: "not-a-tile", W: 1, H: 1}}}); err == nil {
		t.Fatal("a layout for an app that is not on the board was accepted")
	}
	if err := f.service.Patch(ctx, boardID, boards.Update{FontScale: &scale}); err != nil {
		t.Fatalf("zoom: %v", err)
	}
	if view, _ := f.service.View(ctx, boardID); view.FontScale != 1.5 {
		t.Fatalf("font scale = %v", view.FontScale)
	}
}

func TestAddTilesPinsAppsSideBySideAndRefusesBoards(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")
	var appIDs []string
	for _, title := range []string{"Notes", "Chart"} {
		app, err := f.apps.PublishLoopApp(ctx, "", title, "<p>"+title+"</p>", "")
		if err != nil {
			t.Fatalf("publish %s: %v", title, err)
		}
		appIDs = append(appIDs, app.ID)
	}

	// Adding twice keeps the tiles where they are.
	for range 2 {
		if err := f.service.AddTiles(ctx, boardID, appIDs, []loops.Loop{f.loop}); err != nil {
			t.Fatalf("add tiles: %v", err)
		}
	}
	tiles := f.tiles(t, boardID)
	if len(tiles) != 3 {
		t.Fatalf("tiles = %+v", tiles)
	}
	if tiles[0].X != 0 || tiles[0].W != 3 || tiles[1].X != 3 || tiles[1].Y != 0 || tiles[2].Y != 3 || tiles[2].Loop == nil {
		t.Fatalf("tiles overlap or are out of order: %+v", tiles)
	}

	if err := f.service.AddTiles(ctx, boardID, []string{f.board(t, "Inner")}, nil); err == nil {
		t.Fatal("a board was accepted as a tile")
	}
	if err := f.service.RemoveTile(ctx, boardID, appIDs[0]); err != nil {
		t.Fatalf("remove tile: %v", err)
	}
	if tiles := f.tiles(t, boardID); len(tiles) != 2 {
		t.Fatalf("tiles after remove = %+v", tiles)
	}
	if _, err := f.store.GetApp(ctx, appIDs[0]); err != nil {
		t.Fatalf("removing a tile deleted its app: %v", err)
	}
}

func TestDeletingABoardKeepsTheLoopAppForTheNextBoard(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")
	f.assign(t, f.loop, boardID)
	first := f.publish(t, "run-1", boards.PublishInput{HTML: "<p>v1</p>"})

	if err := f.apps.DeleteApp(ctx, boardID); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	if _, err := f.service.View(ctx, boardID); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("deleted board still has a view: %v", err)
	}
	if _, err := f.service.Publish(ctx, f.loop, "run-2", "", boards.PublishInput{HTML: "<p>v2</p>"}); err == nil {
		t.Fatal("expected publish to fail once no boards remain")
	}

	f.assign(t, f.loop, f.board(t, "Next"))
	if again := f.publish(t, "run-3", boards.PublishInput{HTML: "<p>v2</p>"}); again.App.ID != first.App.ID || again.App.CurrentVersion != 2 {
		t.Fatalf("app after reassign = %+v", again.App)
	}
}

func TestPurgeOrphansFreesCellsAndReleasesThePublishedApp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")
	waiting := f.saveLoop(t, "Never ran")
	f.assign(t, f.loop, boardID)
	f.assign(t, waiting, boardID)
	published := f.publish(t, "run-1", boards.PublishInput{HTML: "<p>v1</p>"})
	waitingAppID := f.loopApp(t, waiting).AppID

	for _, loop := range []loops.Loop{f.loop, waiting} {
		loop.Status = loops.StatusDeleted
		if err := f.store.SaveLoop(loop); err != nil {
			t.Fatalf("delete loop: %v", err)
		}
	}
	f.service.PurgeOrphans(ctx)

	if tiles := f.tiles(t, boardID); len(tiles) != 0 {
		t.Fatalf("orphan tiles survived: %+v", tiles)
	}
	// The published app stays in the library as an ordinary app; the one that
	// never ran had nothing to keep.
	if app, err := f.store.GetApp(ctx, published.App.ID); err != nil || app.LoopID != "" {
		t.Fatalf("released app = %+v (%v)", app, err)
	}
	if _, err := f.store.GetApp(ctx, waitingAppID); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("unpublished app of a deleted loop survived: %v", err)
	}

	fresh := f.saveLoop(t, "Fresh")
	f.assign(t, fresh, boardID)
	if tile := f.tiles(t, boardID)[0]; tile.X != 0 || tile.Y != 0 {
		t.Fatalf("fresh tile did not take the freed top spot: %+v", tile)
	}
}

func TestBoardsBelongToTheActiveWorkspace(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boardID := f.board(t, "Desk")

	f.workspace = "cloud-a"
	if listed, err := f.service.ListBoards(ctx); err != nil || len(listed) != 0 {
		t.Fatalf("a cloud workspace lists local boards: %+v (%v)", listed, err)
	}
	if _, err := f.service.View(ctx, boardID); !errors.Is(err, storage.ErrAppNotFound) {
		t.Fatalf("a cloud workspace opened a local board: %v", err)
	}
	if err := f.service.ValidateBoardIDs(ctx, []string{boardID}); err == nil {
		t.Fatal("a cloud workspace accepted a local board id")
	}

	f.workspace = storage.LocalWorkspaceID
	if listed, err := f.service.ListBoards(ctx); err != nil || !slices.ContainsFunc(listed, func(board storage.App) bool { return board.ID == boardID }) {
		t.Fatalf("local boards = %+v (%v)", listed, err)
	}
}

func TestPublishRejectsFullDocuments(t *testing.T) {
	f := newFixture(t)
	f.assign(t, f.loop, f.board(t, "Desk"))
	_, err := f.service.Publish(context.Background(), f.loop, "run-1", "", boards.PublishInput{HTML: "<!doctype html><html><body>hi</body></html>"})
	if err == nil || !strings.Contains(err.Error(), "fragment") {
		t.Fatalf("expected fragment error, got %v", err)
	}
}

func TestTileFeedbackReachesTheNextRunAndClearsOnPublish(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.assign(t, f.loop, f.board(t, "Desk"))
	published := f.publish(t, "run-1", boards.PublishInput{HTML: `<div style="height:100vh;position:fixed">x</div>`})
	if len(published.Warnings) < 2 {
		t.Fatalf("expected lint warnings for viewport units + fixed position, got %v", published.Warnings)
	}
	appID := published.App.ID

	// The first version has no proven tile size yet, so empty space is not a
	// correction signal; hard rendering failures are.
	if err := f.service.ReportTile(ctx, appID, boards.Feedback{
		Error:  "ReferenceError: foo is not defined",
		Layout: `{"dead_space_pct":40,"overflow_px":0,"clipped":1,"img_errors":2}`,
	}); err != nil {
		t.Fatalf("report: %v", err)
	}
	section := f.service.LoopPromptExtra(f.loop, loops.Run{})
	if strings.Contains(section, "40% of the tile is empty") {
		t.Fatalf("first version must not optimize for empty space:\n%s", section)
	}
	for _, want := range []string{"Current: version 1", "clip their content", "2 image(s) failed to load", "foo is not defined"} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt missing %q:\n%s", want, section)
		}
	}

	f.publish(t, "run-2", boards.PublishInput{HTML: "<p>v2</p>"})
	if loopApp := f.loopApp(t, f.loop); loopApp.LastLayout != "" || loopApp.LastError != "" {
		t.Fatalf("feedback not cleared on publish: %+v", loopApp)
	}
	if section := f.service.LoopPromptExtra(f.loop, loops.Run{}); strings.Contains(section, "Board rendering feedback") {
		t.Fatalf("healthy tile still carries telemetry text:\n%s", section)
	}

	if err := f.service.ReportTile(ctx, appID, boards.Feedback{Layout: `{"dead_space_pct":40}`}); err != nil {
		t.Fatalf("report after second publish: %v", err)
	}
	if section := f.service.LoopPromptExtra(f.loop, loops.Run{}); !strings.Contains(section, "40% of the tile is empty") {
		t.Fatalf("established versions should surface empty-space telemetry:\n%s", section)
	}
}

func TestPromptSectionStatesOnlyTheBoardAppContract(t *testing.T) {
	f := newFixture(t)
	section := boards.PromptSection(f.loop, storage.LoopApp{LastError: "boom"}, storage.App{Title: "Open PRs", CurrentVersion: 3})
	for _, want := range []string{
		"## Board App Runtime",
		boards.AppFilePath(f.loop),
		"publish_app",
		"A list or feed is fine",
		"boom",
		"create or overwrite it this run",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("prompt section missing %q:\n%s", want, section)
		}
	}
	for _, reject := range []string{
		"Visual Creation Suite",
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
		"AGENTS.md",
		"design guide next to it",
	} {
		if strings.Contains(section, reject) {
			t.Fatalf("prompt must expose only the board app contract; found %q:\n%s", reject, section)
		}
	}
}
