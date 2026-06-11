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
	if len(catalog.Skills) != 2 {
		t.Fatalf("skills = %#v", catalog.Skills)
	}
	got := map[string]string{}
	for _, skill := range catalog.Skills {
		got[skill.Name] = skill.Description
	}
	if got["alpha"] != "Alpha tasks" || got["beta"] != "Beta tasks" {
		t.Fatalf("unexpected skills: %#v", catalog.Skills)
	}
	prompt := catalog.Prompt()
	for _, want := range []string{"<available_skills>", "<name>alpha</name>", "<name>beta</name>", "SKILL.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
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

func TestInstallDefaultsRefreshesManagedSkills(t *testing.T) {
	root := t.TempDir()
	if err := InstallDefaults(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ManagedRoot(root), "jazmem", "SKILL.md")
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
		t.Fatalf("managed skill was not refreshed:\n%s", data)
	}
}

func TestLoadIncludesManagedDefaultsAndUserOverrides(t *testing.T) {
	root := t.TempDir()
	if err := InstallDefaults(root); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, root, "jazmem", "jazmem", "Custom memory skill")

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
	if got["jazmem"].Description != "Custom memory skill" {
		t.Fatalf("user skill did not override managed default: %#v", got["jazmem"])
	}
	if !strings.HasPrefix(got["make-interfaces-feel-better"].Path, ManagedRoot(root)) {
		t.Fatalf("default skill path = %q, want managed root", got["make-interfaces-feel-better"].Path)
	}
}

func writeSkill(t *testing.T, root, dir, name, description string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "skills", dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+description+"\n---\nbody")
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
