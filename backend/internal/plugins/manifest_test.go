package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/skills"
)

func TestManifestLayoutsDriveInventoryAndAgentSkills(t *testing.T) {
	for _, test := range []struct {
		layout  string
		body    string
		display string
		want    []string
	}{
		{"plugin.json", portable(t, "acme.tools", map[string]any{"displayName": "Portable tools"}), "Portable tools", []string{"acme.tools:default"}},
		{".claude-plugin/plugin.json", `{"name":"acme.tools","displayName":"Claude tools","skills":["./extra/","./single/",".","./skills/"],"hooks":{"command":"ignored"}}`, "Claude tools", []string{"acme.tools:default", "acme.tools:extra", "acme.tools:single", "acme.tools:root", "acme.tools:skills-root"}},
		{".codex-plugin/plugin.json", `{"name":"acme.tools","interface":{"displayName":"Codex tools"},"skills":"./extra/","mcpServers":"./custom-mcp.json","apps":"./.app.json"}`, "Codex tools", []string{"acme.tools:default", "acme.tools:extra"}},
	} {
		t.Run(test.layout, func(t *testing.T) {
			layout := runtimefiles.New(t.TempDir())
			store := New(layout)
			files := map[string]string{
				"bundle/" + test.layout:                            test.body,
				"bundle/skills/default/SKILL.md":                   "---\nname: default\ndescription: Default skill\n---\n",
				"bundle/skills/SKILL.md":                           "---\nname: skills-root\ndescription: Explicit directory skill\n---\n",
				"bundle/skills/README.md":                          "Not a skill directory",
				"bundle/skills/default/references/nested/SKILL.md": "---\nname: hidden\ndescription: Nested reference\n---\n",
				"bundle/extra/more/SKILL.md":                       "---\nname: extra\ndescription: Extra skill\n---\n",
				"bundle/single/SKILL.md":                           "---\nname: single\ndescription: Single skill\n---\n",
				"bundle/SKILL.md":                                  "---\nname: root\ndescription: Root skill\n---\n",
				"bundle/commands/unrequested/SKILL.md":             "---\nname: command\ndescription: Unsupported component\n---\n",
				"bundle/custom-mcp.json":                           `{"mcpServers":{}}`,
				"bundle/.app.json":                                 `{not read}`,
			}
			installed, err := store.Install(archive(t, files))
			if err != nil || len(installed) != 1 {
				t.Fatalf("install = %+v, %v", installed, err)
			}
			if installed[0].DisplayName != test.display || !slices.Equal(installed[0].Skills, test.want) || len(installed[0].Warnings) != 0 {
				t.Fatalf("inventory = %+v, want %s with %v", installed[0], test.display, test.want)
			}
			catalog, err := skills.Load(layout.Root)
			if err != nil || len(catalog.Skills) != len(test.want) {
				t.Fatalf("catalog = %+v, %v", catalog, err)
			}
			for _, skill := range catalog.Skills {
				if skill.Plugin != "acme.tools" || !strings.HasPrefix(skill.Path, filepath.Join(layout.Plugins, "acme.tools")+string(filepath.Separator)) || !strings.Contains(catalog.Prompt(), skill.Path) {
					t.Fatalf("skill lost its identity or original path: %+v", skill)
				}
				if _, err := os.Stat(skill.Path); err != nil {
					t.Fatal(err)
				}
			}
			definition, err := manifest.Read(filepath.Join(layout.Plugins, "acme.tools"))
			if err != nil || definition.ManifestPath != test.layout {
				t.Fatalf("selected manifest = %+v, %v", definition, err)
			}
			if test.layout == ".codex-plugin/plugin.json" && string(definition.MCPServers) != `"./custom-mcp.json"` {
				t.Fatalf("MCP declaration lost: %s", definition.MCPServers)
			}
		})
	}
}

func TestManifestPrecedenceAndUnknownCoreFields(t *testing.T) {
	for _, hasRoot := range []bool{true, false} {
		store := New(runtimefiles.New(t.TempDir()))
		files := map[string]string{
			"bundle/.claude-plugin/plugin.json": `{"name":"claude","displayName":"Claude fallback"}`,
			"bundle/.codex-plugin/plugin.json":  `{"name":"codex","interface":{"displayName":"Codex fallback"}}`,
		}
		want := "Claude fallback"
		if hasRoot {
			files["bundle/plugin.json"] = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","title":"Ignored old title","page":{"view":"ignored","file":"ignored.json"},"extensions":{"other.client":false}}`
			want = "portable"
		}
		installed, err := store.Install(archive(t, files))
		if err != nil || installed[0].DisplayName != want || installed[0].Page != nil {
			t.Fatalf("precedence = %+v, %v", installed, err)
		}
		if hasRoot && len(installed[0].Warnings) != 2 {
			t.Fatalf("unknown root fields were not reported: %+v", installed[0])
		}
	}
	store := New(runtimefiles.New(t.TempDir()))
	if _, err := store.Install(archive(t, map[string]string{"bundle/plugin.json": `{"name":"old-shape"}`, "bundle/.claude-plugin/plugin.json": `{"name":"fallback"}`})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid root fell back to native: %v", err)
	}
}

func TestPortableManifestValidationAtInstall(t *testing.T) {
	for _, test := range []struct {
		field string
		value any
	}{
		{"$schema", nil}, {"$schema", "https://example.com/unknown"},
		{"name", "Uppercase"}, {"name", "trailing-"}, {"name", "double--dash"}, {"name", "double..dot"}, {"name", strings.Repeat("a", 65)},
		{"version", 3}, {"description", false}, {"author", "invalid"}, {"author", map[string]any{"name": false}},
		{"homepage", false}, {"repository", []string{}}, {"license", false}, {"keywords", []any{"valid", false}},
		{"extensions", []any{}}, {"extensions", false},
		{"extensions", map[string]any{"chat.jaz": map[string]any{"unknown": true}}},
		{"extensions", map[string]any{"chat.jaz": map[string]any{"page": map[string]any{"view": "status", "file": "status.json", "icon": "legacy"}}}},
	} {
		t.Run(test.field, func(t *testing.T) {
			fields := map[string]any{"$schema": manifest.Schema, "name": "valid"}
			fields[test.field] = test.value
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			store := New(runtimefiles.New(t.TempDir()))
			if _, err := store.Install(archive(t, map[string]string{"plugin.json": string(data)})); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted invalid %s=%v: %v", test.field, test.value, err)
			}
		})
	}
}

func TestPluginFilesCannotResolveOutsideThePackage(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	if _, err := store.Install(archive(t, map[string]string{
		"bundle/plugin.json":            portable(t, "contained", nil),
		"bundle/skills/inside/SKILL.md": "---\nname: inside\ndescription: Skill inside\n---\n",
		"bundle/views/status.html":      "<p>inside</p>",
	})); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(layout.Plugins, "contained")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("---\nname: outside\ndescription: Must not load\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"skills/inside/SKILL.md", "views/status.html"} {
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := skills.Load(layout.Root)
	if err != nil || len(catalog.Skills) != 0 {
		t.Fatalf("external skill loaded: %+v, %v", catalog, err)
	}
	if _, err := store.View("contained/status"); err == nil {
		t.Fatal("external view loaded")
	}
	if err := os.Remove(filepath.Join(root, "plugin.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "plugin.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Read(root); err == nil {
		t.Fatal("external manifest loaded")
	}
}

func TestPluginSkillNamespacesPreserveCollisionsAndOriginalPaths(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	if _, err := store.Install(archive(t, map[string]string{
		"tenders/plugin.json":                     portable(t, "tenders", nil),
		"tenders/skills/query-sheet/SKILL.md":     "---\nname: query-sheet\ndescription: Tender query\n---\n",
		"tenders/skills/bid-preparation/SKILL.md": "---\nname: bid-preparation\ndescription: Tender bid\n---\n",
		"other.tools/plugin.json":                 portable(t, "other.tools", nil),
		"other.tools/skills/query-sheet/SKILL.md": "---\nname: query-sheet\ndescription: Other query\n---\n",
	})); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"tenders:query-sheet":     filepath.Join(layout.Plugins, "tenders", "skills", "query-sheet", "SKILL.md"),
		"tenders:bid-preparation": filepath.Join(layout.Plugins, "tenders", "skills", "bid-preparation", "SKILL.md"),
		"other.tools:query-sheet": filepath.Join(layout.Plugins, "other.tools", "skills", "query-sheet", "SKILL.md"),
	}
	for _, name := range []string{"query-sheet", "tenders", "other"} {
		path := filepath.Join(layout.UserSkills, name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: Standalone skill\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
		want[name] = path
	}
	catalog, err := skills.Load(layout.Root)
	if err != nil || len(catalog.Skills) != 6 {
		t.Fatalf("catalogue lost a colliding skill: %+v %v", catalog, err)
	}
	for _, skill := range catalog.Skills {
		if skill.Path != want[skill.Name] || !strings.Contains(catalog.Prompt(), "<name>"+skill.Name+"</name>") || !strings.Contains(catalog.Prompt(), skill.Path) {
			t.Fatalf("catalogue name or original path changed: %+v", skill)
		}
	}
	installed, err := store.List()
	if err != nil || !slices.Equal(installed[0].Skills, []string{"other.tools:query-sheet"}) || !slices.Equal(installed[1].Skills, []string{"tenders:bid-preparation", "tenders:query-sheet"}) {
		t.Fatalf("Settings inventory = %+v %v", installed, err)
	}
}
