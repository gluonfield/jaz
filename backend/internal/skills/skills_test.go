package skills

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func TestSyncRemoteInstallsManifestSkills(t *testing.T) {
	root := t.TempDir()
	archive := skillArchive(t, "create-animated-video", map[string]string{
		"SKILL.md":                          "---\nname: create-animated-video\ndescription: Video skill\n---\nbody",
		"template/scripts/export-video.mjs": "export",
		"references/finalize_playback.md":   "finalize",
	})
	sum := sha256.Sum256(archive)
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(RemoteManifest{Skills: []RemoteSkill{{
				Name:       "create-animated-video",
				Version:    "2026.06.20",
				ArchiveURL: serverURL + "/create-animated-video.zip",
				SHA256:     hex.EncodeToString(sum[:]),
			}}})
		case "/create-animated-video.zip":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	if err := SyncRemote(t.Context(), root, RemoteSyncConfig{ManifestURL: server.URL + "/manifest.json"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(UserRoot(root), "create-animated-video", "template", "scripts", "export-video.mjs")
	if data, err := os.ReadFile(path); err != nil || string(data) != "export" {
		t.Fatalf("synced file = %q, %v", data, err)
	}
	state, err := loadSyncState(root)
	if err != nil {
		t.Fatal(err)
	}
	if state["create-animated-video"].Source != firstPartySkillSource {
		t.Fatalf("sync state = %#v", state)
	}
}

func TestSyncRemoteVerifiesManifestHash(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{"version":"test","skills":[]}` + "\n")
	sum := sha256.Sum256(manifest)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifest)
	}))
	defer server.Close()

	if err := SyncRemote(t.Context(), root, RemoteSyncConfig{
		ManifestURL:    server.URL + "/manifest.json",
		ManifestSHA256: hex.EncodeToString(sum[:]),
	}); err != nil {
		t.Fatal(err)
	}
	err := SyncRemote(t.Context(), root, RemoteSyncConfig{
		ManifestURL:    server.URL + "/manifest.json",
		ManifestSHA256: strings.Repeat("0", sha256.Size*2),
	})
	if err == nil || !strings.Contains(err.Error(), "manifest sha256 mismatch") {
		t.Fatalf("err = %v, want manifest sha mismatch", err)
	}
}

func TestSyncRemoteRefreshesManagedSkill(t *testing.T) {
	root := t.TempDir()
	body := "first"
	var archive []byte
	var sha string
	refreshArchive := func() {
		archive = skillArchive(t, "alpha", map[string]string{
			"SKILL.md": "---\nname: alpha\ndescription: Alpha skill\n---\n" + body,
		})
		sum := sha256.Sum256(archive)
		sha = hex.EncodeToString(sum[:])
	}
	refreshArchive()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(RemoteManifest{Skills: []RemoteSkill{{
				Name:       "alpha",
				ArchiveURL: "http://" + r.Host + "/alpha.zip",
				SHA256:     sha,
			}}})
		case "/alpha.zip":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := SyncRemote(t.Context(), root, RemoteSyncConfig{ManifestURL: server.URL + "/manifest.json"}); err != nil {
		t.Fatal(err)
	}
	body = "second"
	refreshArchive()
	if err := SyncRemote(t.Context(), root, RemoteSyncConfig{ManifestURL: server.URL + "/manifest.json"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(UserRoot(root), "alpha", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "second") {
		t.Fatalf("managed skill = %q, %v", data, err)
	}
}

func TestSyncRemoteLeavesUserOwnedSkill(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(UserRoot(root), "create-animated-video", "SKILL.md"), "---\nname: create-animated-video\ndescription: User skill\n---\nuser-owned")
	archive := skillArchive(t, "create-animated-video", map[string]string{
		"SKILL.md": "---\nname: create-animated-video\ndescription: Video skill\n---\nremote",
	})
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(RemoteManifest{Skills: []RemoteSkill{{
				Name:       "create-animated-video",
				ArchiveURL: "http://" + r.Host + "/create-animated-video.zip",
				SHA256:     hex.EncodeToString(sum[:]),
			}}})
		case "/create-animated-video.zip":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := SyncRemote(t.Context(), root, RemoteSyncConfig{ManifestURL: server.URL + "/manifest.json"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(UserRoot(root), "create-animated-video", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "user-owned") {
		t.Fatalf("user-owned skill = %q, %v", data, err)
	}
}

func TestInstallMissingToCopiesMissingSkillsAndLeavesExistingDirs(t *testing.T) {
	root := t.TempDir()
	dst := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Alpha tasks")
	writeFile(t, filepath.Join(root, "skills", "alpha", "references", "guide.md"), "guide")
	script := filepath.Join(root, "skills", "alpha", "scripts", "run.sh")
	writeFile(t, script, "#!/bin/sh\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := InstallMissingTo(root, dst); err != nil {
		t.Fatal(err)
	}

	if data, err := os.ReadFile(filepath.Join(dst, "alpha", "references", "guide.md")); err != nil || string(data) != "guide" {
		t.Fatalf("copied reference = %q, %v", data, err)
	}
	info, err := os.Stat(filepath.Join(dst, "alpha", "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("copied script mode = %v", info.Mode().Perm())
	}

	writeFile(t, filepath.Join(root, "skills", "alpha", "SKILL.md"), "---\nname: alpha\ndescription: Updated\n---\nnew body")
	if err := InstallMissingTo(root, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Updated") {
		t.Fatalf("existing skill was overwritten:\n%s", data)
	}
}

func TestInstallMissingToSkipsExistingSkillConflictsAndLeavesOrphans(t *testing.T) {
	root := t.TempDir()
	dst := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Alpha tasks")
	writeSkill(t, root, "stale", "stale", "Stale tasks")
	writeFile(t, filepath.Join(dst, "alpha", "SKILL.md"), "user-owned")
	writeFile(t, filepath.Join(dst, "orphan", "SKILL.md"), "user-owned orphan")

	if err := InstallMissingTo(root, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "stale", "SKILL.md")); err != nil {
		t.Fatalf("stale skill missing before source removal: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(root, "skills", "stale")); err != nil {
		t.Fatal(err)
	}
	if err := InstallMissingTo(root, dst); err != nil {
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
	if _, err := os.Stat(filepath.Join(dst, "stale", "SKILL.md")); err != nil {
		t.Fatalf("additive sync should leave old skills in place: %v", err)
	}
}

func TestInstallMissingToToleratesConcurrentMissingSkillCopies(t *testing.T) {
	root := t.TempDir()
	dst := t.TempDir()
	writeSkill(t, root, "alpha", "alpha", "Alpha tasks")

	errs := make(chan error, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- InstallMissingTo(root, dst)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dst, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Alpha tasks") {
		t.Fatalf("skill was not copied:\n%s", data)
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

func skillArchive(t *testing.T, name string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for rel, content := range files {
		w, err := zw.Create(filepath.ToSlash(filepath.Join(name, rel)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
