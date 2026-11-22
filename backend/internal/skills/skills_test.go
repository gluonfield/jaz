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

func TestLoadFilteredSkipsDisabledPluginSkills(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "plugins", "alpha", "plugin.json"), `{"name":"alpha","description":"Alpha"}`)
	writeFile(t, filepath.Join(root, "plugins", "alpha", "skills", "one", "SKILL.md"), "---\nname: one\ndescription: One\n---\nbody")
	writeFile(t, filepath.Join(root, "plugins", "beta", "skills", "two", "SKILL.md"), "---\nname: two\ndescription: Two\n---\nbody")

	catalog, err := LoadFiltered(root, func(name string) bool { return name == "beta" })
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != "beta:two" {
		t.Fatalf("filtered skills = %#v", catalog.Skills)
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
