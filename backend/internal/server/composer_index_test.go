package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListWorkspaceFiles(t *testing.T) {
	workspace := t.TempDir()
	for _, dir := range []string{"repo/src", "repo/node_modules/pkg"} {
		if err := os.MkdirAll(filepath.Join(workspace, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "repo", "src", "main.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/workspace/files?path=repo", nil)
	res := httptest.NewRecorder()
	(&Server{Workspace: workspace}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Root      string `json:"root"`
		Truncated bool   `json:"truncated"`
		Entries   []struct {
			Path string `json:"path"`
			Dir  bool   `json:"dir"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(body.Root) || filepath.Base(body.Root) != "repo" {
		t.Fatalf("root = %q, want absolute path ending in repo", body.Root)
	}
	got := make(map[string]bool, len(body.Entries))
	for _, entry := range body.Entries {
		got[entry.Path] = entry.Dir
	}
	if dir, ok := got["src"]; !ok || !dir {
		t.Fatalf("entries missing dir src: %v", got)
	}
	if dir, ok := got["src/main.go"]; !ok || dir {
		t.Fatalf("entries missing file src/main.go: %v", got)
	}
	if _, ok := got["node_modules"]; ok {
		t.Fatalf("entries should skip node_modules: %v", got)
	}

	escape := httptest.NewRequest(http.MethodGet, "/v1/workspace/files?path=../outside", nil)
	res = httptest.NewRecorder()
	(&Server{Workspace: workspace}).Handler().ServeHTTP(res, escape)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("escape path status = %d, want 400", res.Code)
	}
}

func TestListSkills(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	frontmatter := "---\nname: alpha\ndescription: First skill\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(frontmatter), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	res := httptest.NewRecorder()
	(&Server{Root: root}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	// Decoding into lowercase-keyed structs pins the Skill JSON tags.
	var body struct {
		Skills []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Path        string `json:"path"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Skills) != 1 || body.Skills[0].Name != "alpha" || body.Skills[0].Description != "First skill" {
		t.Fatalf("skills = %+v, want alpha/First skill", body.Skills)
	}
	if !filepath.IsAbs(body.Skills[0].Path) {
		t.Fatalf("skill path = %q, want absolute", body.Skills[0].Path)
	}
}
