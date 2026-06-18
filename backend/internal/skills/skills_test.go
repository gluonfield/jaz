package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadScansJazSkillsOnly(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Alpha tasks")
	writeSkill(t, root, ".system/beta", "beta", "Beta tasks")
	writeSkill(t, root, "duplicate", "alpha", "Duplicate")
	writeFile(t, filepath.Join(root, "skills", "bad", "SKILL.md"), "---\nname: bad\n---\nbody")
	writeFile(t, filepath.Join(root, "skills", "unnamed", "SKILL.md"), "---\ndescription: Missing name\n---\nbody")

	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Root != filepath.Join(root, "skills") {
		t.Fatalf("root = %q", catalog.Root)
	}
	if len(catalog.Skills) != 1 {
		t.Fatalf("skills = %#v", catalog.Skills)
	}
	got := map[string]string{}
	for _, skill := range catalog.Skills {
		got[skill.Name] = skill.Description
	}
	if got["alpha"] != "Alpha tasks" {
		t.Fatalf("unexpected skills: %#v", catalog.Skills)
	}
	prompt := catalog.Prompt()
	for _, want := range []string{"<available_skills>", "<name>alpha</name>", "SKILL.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "<name>beta</name>") {
		t.Fatalf("prompt includes hidden skill:\n%s", prompt)
	}
}

func TestLoadMissingRootIsEmptyCatalog(t *testing.T) {
	root := t.TempDir()
	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Root != filepath.Join(root, "skills") || len(catalog.Skills) != 0 || catalog.Prompt() != "" {
		t.Fatalf("unexpected catalog %#v", catalog)
	}
}

func TestLoadForWorkspaceMergesLocalSkillDirs(t *testing.T) {
	root := t.TempDir()
	workspace := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Global alpha")
	writeSkill(t, root, "global", "global", "Global only")
	writeWorkspaceSkill(t, workspace, ".claude", "local", "local", "Claude local")
	writeWorkspaceSkill(t, workspace, ".codex", "alpha", "alpha", "Codex override")
	writeWorkspaceSkill(t, workspace, ".agents", "agent", "agent", "Agent local")
	writeWorkspaceSkill(t, workspace, ".jaz", "local", "local", "Jaz override")

	catalog, err := LoadForWorkspace(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Skill{}
	for _, skill := range catalog.Skills {
		got[skill.Name] = skill
	}
	for name, description := range map[string]string{
		"alpha":  "Codex override",
		"global": "Global only",
		"local":  "Jaz override",
		"agent":  "Agent local",
	} {
		if got[name].Description != description {
			t.Fatalf("%s = %#v, want %q; catalog = %#v", name, got[name], description, catalog.Skills)
		}
	}
	if !strings.HasPrefix(got["local"].Path, filepath.Join(workspace, ".jaz", "skills")) {
		t.Fatalf("local skill path = %q", got["local"].Path)
	}
}

func TestInstallDefaultsRefreshesDefaultSkills(t *testing.T) {
	root := t.TempDir()
	if err := InstallDefaults(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(UserRoot(root), "jazmem", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default skill missing: %v", err)
	}
	if err := os.WriteFile(path, []byte("---\nname: stale\ndescription: stale\n---\nstale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallDefaults(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "name: stale") || !strings.Contains(string(data), "name: jazmem") {
		t.Fatalf("default skill was not refreshed:\n%s", data)
	}
}

func TestInstallDefaultsKeepsCustomSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "custom", "custom", "Custom skill")

	if err := InstallDefaults(root); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(UserRoot(root), "make-interfaces-feel-better", "SKILL.md")); err != nil {
		t.Fatalf("default skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(UserRoot(root), "custom", "SKILL.md")); err != nil {
		t.Fatalf("custom skill missing: %v", err)
	}
}

func TestLoadIncludesDefaultSkills(t *testing.T) {
	root := t.TempDir()
	if err := InstallDefaults(root); err != nil {
		t.Fatal(err)
	}

	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Skill{}
	for _, skill := range catalog.Skills {
		got[skill.Name] = skill
	}
	for _, name := range []string{"jazmem", "make-interfaces-feel-better", "thermo-nuclear-code-quality-review"} {
		if got[name].Name == "" {
			t.Fatalf("missing skill %q from %#v", name, catalog.Skills)
		}
	}
	for _, skill := range got {
		if !strings.HasPrefix(skill.Path, UserRoot(root)) {
			t.Fatalf("default skill path = %q, want user root", skill.Path)
		}
	}
}

func TestSyncToCopiesAndRefreshesManagedSkills(t *testing.T) {
	root := t.TempDir()
	dst := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Alpha tasks")
	writeFile(t, filepath.Join(root, "skills", "alpha", "references", "guide.md"), "guide")
	script := filepath.Join(root, "skills", "alpha", "scripts", "run.sh")
	writeFile(t, script, "#!/bin/sh\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := SyncTo(root, dst); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dst, "alpha", managedMarker)); err != nil {
		t.Fatalf("managed marker missing: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "alpha", "references", "guide.md")); err != nil || string(data) != "guide" {
		t.Fatalf("copied reference = %q, %v", data, err)
	}
	if info, err := os.Stat(filepath.Join(dst, "alpha", "scripts", "run.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("copied script mode = %v, %v", info.Mode().Perm(), err)
	}

	writeFile(t, filepath.Join(root, "skills", "alpha", "SKILL.md"), "---\nname: alpha\ndescription: Updated\n---\nnew body")
	if err := SyncTo(root, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Updated") {
		t.Fatalf("managed copy was not refreshed:\n%s", data)
	}
}

func TestSyncToSkipsUserOwnedSkillConflictsAndLeavesOrphans(t *testing.T) {
	root := t.TempDir()
	dst := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Alpha tasks")
	writeSkill(t, root, "stale", "stale", "Stale tasks")
	writeFile(t, filepath.Join(dst, "alpha", "SKILL.md"), "user-owned")
	writeFile(t, filepath.Join(dst, "orphan", "SKILL.md"), "user-owned orphan")

	if err := SyncTo(root, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "stale", managedMarker)); err != nil {
		t.Fatalf("managed stale skill missing before source removal: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(root, "skills", "stale")); err != nil {
		t.Fatal(err)
	}
	if err := SyncTo(root, dst); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "user-owned" {
		t.Fatalf("user-owned skill was overwritten:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dst, "orphan", "SKILL.md")); err != nil {
		t.Fatalf("user-owned orphan should stay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "stale", managedMarker)); err != nil {
		t.Fatalf("additive sync should leave old managed skills in place: %v", err)
	}
}

func writeSkill(t *testing.T, root, dir, name, description string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "skills", dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+description+"\n---\nbody")
}

func writeWorkspaceSkill(t *testing.T, workspace, owner, dir, name, description string) {
	t.Helper()
	writeFile(t, filepath.Join(workspace, owner, "skills", dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+description+"\n---\nbody")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
