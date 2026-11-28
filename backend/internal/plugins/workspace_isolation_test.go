package plugins

import (
	"testing"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testgit"
)

func TestPluginsIsolatedAcrossWorkspaces(t *testing.T) {
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, map[string]string{
		"plugin.json":         `{"name":"alpha","displayName":"Alpha","description":"Test plugin"}`,
		"skills/run/SKILL.md": "---\nname: alpha-run\ndescription: Run alpha\n---\n",
		"mcp.json":            `{"mcpServers":{"srv":{"type":"http","url":"https://alpha.example.com/mcp"}}}`,
	})
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)

	layout := runtimefiles.New(t.TempDir())
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	activeWS := storage.LocalWorkspaceID
	catalog := connections.NewCatalog(nil)
	connSvc := connections.NewService(catalog, db, nil, nil, nil)
	store := NewStore(layout, db, catalog).WithWorkspace(func() string { return activeWS })
	svc := NewService(store, db, connSvc, nil, nil, nil)

	ctx := t.Context()

	// Local workspace: add source + install alpha; native plugins also visible.
	if _, err := svc.AddSource(ctx, RepositoryInput{Repository: repo}); err != nil {
		t.Fatalf("local AddSource: %v", err)
	}
	if _, err := svc.Install(ctx, RepositoryInput{Repository: repo}); err != nil {
		t.Fatalf("local Install: %v", err)
	}
	localCatalog, err := svc.List(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(localCatalog.Sources) != 1 {
		t.Fatalf("local sources = %d, want 1", len(localCatalog.Sources))
	}
	if !store.IsEnabled(ctx, "alpha") {
		t.Fatal("alpha should be enabled in local")
	}
	localMCP, err := store.ListMCPServers()
	if err != nil || len(localMCP) != 1 {
		t.Fatalf("local plugin mcp servers = %+v, %v", localMCP, err)
	}

	// Switch to cloudA: must start completely clean (no local plugins, no sources, no native personal connectors).
	activeWS = "cloudA"
	cloudACatalog, err := svc.List(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cloudACatalog.Plugins) != 0 || len(cloudACatalog.Sources) != 0 {
		t.Fatalf("cloudA leaked local plugins/sources: plugins=%d sources=%d", len(cloudACatalog.Plugins), len(cloudACatalog.Sources))
	}
	if added, err := svc.EnsureDefaultSource(ctx); err != nil || added {
		t.Fatalf("cloudA EnsureDefaultSource = added=%v err=%v; want false, nil", added, err)
	}
	if store.IsEnabled(ctx, "alpha") {
		t.Fatal("local alpha reported enabled in cloudA")
	}
	if cloudAMCP, err := store.ListMCPServers(); err != nil || len(cloudAMCP) != 0 {
		t.Fatalf("cloudA plugin mcp servers = %+v, %v; want empty", cloudAMCP, err)
	}

	// Install the same source & plugin ID into cloudA and disable it without affecting local.
	if _, err := svc.AddSource(ctx, RepositoryInput{Repository: repo}); err != nil {
		t.Fatalf("cloudA AddSource: %v", err)
	}
	if _, err := svc.Install(ctx, RepositoryInput{Repository: repo}); err != nil {
		t.Fatalf("cloudA Install: %v", err)
	}
	if _, err := svc.SetEnabled(ctx, "alpha", false); err != nil {
		t.Fatalf("cloudA SetEnabled(false): %v", err)
	}
	if store.IsEnabled(ctx, "alpha") {
		t.Fatal("alpha should be disabled in cloudA")
	}

	// Switch to cloudB: must still be completely empty.
	activeWS = "cloudB"
	cloudBCatalog, err := svc.List(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cloudBCatalog.Plugins) != 0 || len(cloudBCatalog.Sources) != 0 {
		t.Fatalf("cloudB leaked plugins/sources: plugins=%d sources=%d", len(cloudBCatalog.Plugins), len(cloudBCatalog.Sources))
	}

	// Switch back to local: alpha stays installed and enabled, and .workspaces never appears as a plugin.
	activeWS = storage.LocalWorkspaceID
	if !store.IsEnabled(ctx, "alpha") {
		t.Fatal("disabling alpha in cloudA disabled it in local")
	}
	installedLocal, err := store.ListWithContext(ctx)
	if err != nil || len(installedLocal) != 1 || installedLocal[0].ID != "alpha" || !installedLocal[0].Enabled {
		t.Fatalf("local installed plugins = %+v, %v", installedLocal, err)
	}
}
