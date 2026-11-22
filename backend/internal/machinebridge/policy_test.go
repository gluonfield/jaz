package machinebridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPolicyGuardAccessModes(t *testing.T) {
	roDir := t.TempDir()
	rwDir := t.TempDir()
	rwdDir := t.TempDir()

	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "ro", RootPath: roDir, Mode: AccessReadOnly},
		{ID: "rw", RootPath: rwDir, Mode: AccessReadWrite},
		{ID: "rwd", RootPath: rwdDir, Mode: AccessReadWriteDelete},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := guard.Resolve("ro", "notes.md", OpRead); err != nil {
		t.Fatalf("ro read failed: %v", err)
	}
	if _, err := guard.Resolve("ro", "notes.md", OpWrite); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("ro write = %v, want ErrAccessDenied", err)
	}
	if _, err := guard.Resolve("ro", "notes.md", OpDelete); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("ro delete = %v, want ErrAccessDenied", err)
	}
	if _, err := guard.Resolve("rw", "notes.md", OpWrite); err != nil {
		t.Fatalf("rw write failed: %v", err)
	}
	if _, err := guard.Resolve("rw", "notes.md", OpDelete); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("rw delete = %v, want ErrAccessDenied", err)
	}
	if _, err := guard.Resolve("rwd", "notes.md", OpDelete); err != nil {
		t.Fatalf("rwd delete failed: %v", err)
	}
	if _, err := guard.Resolve("ro", "", OpRead); err != nil {
		t.Fatalf("ro read grant root failed: %v", err)
	}
	if _, err := guard.Resolve("rwd", ".", OpDelete); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("rwd delete grant root = %v, want ErrAccessDenied", err)
	}
	if _, err := guard.Resolve("rwd", "", OpWrite); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("rwd write grant root = %v, want ErrAccessDenied", err)
	}
	if _, err := guard.Resolve("missing", "notes.md", OpRead); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unknown grant = %v, want ErrAccessDenied", err)
	}
}

func TestPolicyGuardRejectsTraversalAndWindowsAbuse(t *testing.T) {
	parent := t.TempDir()
	grantDir := filepath.Join(parent, "grant")
	siblingDir := filepath.Join(parent, "grant-evil")
	if err := os.MkdirAll(grantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "work", RootPath: grantDir, Mode: AccessReadWriteDelete},
	})
	if err != nil {
		t.Fatal(err)
	}

	attacks := []string{
		"../secret.txt",
		"sub/../../secret.txt",
		`sub\..\..\secret.txt`,
		"/etc/passwd",
		filepath.Join(siblingDir, "secret.txt"),
		`C:\Windows\System32\drivers\etc\hosts`,
		`C:relative.txt`,
		`\\?\C:\secret.txt`,
		`\\server\share\file.txt`,
		`//server/share/file.txt`,
		"notes.txt:hiddenstream",
		"bad\x00name.txt",
		"CON",
		"sub/nul.txt",
		"COM1",
	}
	for _, attack := range attacks {
		if got, err := guard.Resolve("work", attack, OpRead); !errors.Is(err, ErrPathOutsideGrant) {
			t.Errorf("Resolve(%q) = (%+v, %v), want ErrPathOutsideGrant", attack, got, err)
		}
	}

	// Valid absolute path inside the grant resolves cleanly.
	inside := filepath.Join(grantDir, "docs", "spec.md")
	resolved, err := guard.Resolve("work", inside, OpWrite)
	if err != nil {
		t.Fatalf("valid absolute inside path failed: %v", err)
	}
	if resolved.RelPath != "docs/spec.md" {
		t.Fatalf("resolved.RelPath = %q, want docs/spec.md", resolved.RelPath)
	}
}

func TestPolicyGuardRejectsSymlinkEscapesAndAllowsInternalSymlinks(t *testing.T) {
	parent := t.TempDir()
	grantDir := filepath.Join(parent, "grant")
	outsideDir := filepath.Join(parent, "outside")
	if err := os.MkdirAll(filepath.Join(grantDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	insideFile := filepath.Join(grantDir, "sub", "ok.txt")
	if err := os.WriteFile(insideFile, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Direct symlink to outside file.
	if err := os.Symlink(outsideFile, filepath.Join(grantDir, "file-escape.txt")); err != nil {
		t.Fatal(err)
	}
	// Directory symlink to outside dir.
	if err := os.Symlink(outsideDir, filepath.Join(grantDir, "dir-escape")); err != nil {
		t.Fatal(err)
	}
	// Dangling symlink pointing to non-existent file outside grant (write escape vector).
	if err := os.Symlink(filepath.Join(outsideDir, "new-evil.txt"), filepath.Join(grantDir, "dangling-escape.txt")); err != nil {
		t.Fatal(err)
	}
	// Two-hop symlink chain escaping outside grant.
	if err := os.Symlink("dir-escape", filepath.Join(grantDir, "hop1")); err != nil {
		t.Fatal(err)
	}
	// Symlink loop inside grant.
	if err := os.Symlink("loop-b", filepath.Join(grantDir, "loop-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop-a", filepath.Join(grantDir, "loop-b")); err != nil {
		t.Fatal(err)
	}
	// Safe internal symlink staying within grant.
	if err := os.Symlink(filepath.Join("sub", "ok.txt"), filepath.Join(grantDir, "safe-link.txt")); err != nil {
		t.Fatal(err)
	}

	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "work", RootPath: grantDir, Mode: AccessReadWriteDelete},
	})
	if err != nil {
		t.Fatal(err)
	}

	escapes := []struct {
		path string
		op   Operation
	}{
		{"file-escape.txt", OpRead},
		{"dir-escape/secret.txt", OpRead},
		{"dir-escape/brand-new.txt", OpWrite},
		{"dangling-escape.txt", OpWrite},
		{"hop1/secret.txt", OpRead},
		{"hop1/new.txt", OpWrite},
		{"loop-a", OpRead},
	}
	for _, tc := range escapes {
		if got, err := guard.Resolve("work", tc.path, tc.op); !errors.Is(err, ErrSymlinkEscape) {
			t.Errorf("Resolve(%q, %s) = (%+v, %v), want ErrSymlinkEscape", tc.path, tc.op, got, err)
		}
	}

	safe, err := guard.Resolve("work", "safe-link.txt", OpRead)
	if err != nil {
		t.Fatalf("internal safe symlink rejected: %v", err)
	}
	wantReal, err := filepath.EvalSymlinks(insideFile)
	if err != nil {
		t.Fatal(err)
	}
	if safe.RealPath != wantReal {
		t.Fatalf("safe.RealPath = %q, want %q", safe.RealPath, wantReal)
	}
}

func TestWithinPathWindowsSemantics(t *testing.T) {
	cases := []struct {
		root   string
		target string
		want   bool
	}{
		{`C:\Users\Alice\Work`, `C:\Users\Alice\Work`, true},
		{`C:\Users\Alice\Work`, `c:\users\alice\work\sub\file.txt`, true},
		{`C:\Users\Alice\Work\`, `C:\Users\Alice\Work\a.txt`, true},
		{`C:\Users\Alice\Work`, `C:\Users\Alice\Work2\file.txt`, false},
		{`C:\Users\Alice\Work`, `D:\Users\Alice\Work\file.txt`, false},
		{`C:\Users\Alice\Work`, `C:\Users\Alice\secret.txt`, false},
	}
	for _, tc := range cases {
		if got := withinPath(tc.root, tc.target, true, '\\'); got != tc.want {
			t.Errorf("withinPath(%q, %q) = %v, want %v", tc.root, tc.target, got, tc.want)
		}
	}
}

func TestValidateStageCommitRequestsAndPreconditions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "calc.xlsx")
	body := []byte("sheet-v1")
	if err := os.WriteFile(target, body, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	mtime := info.ModTime().UnixMilli()
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])

	guard, err := NewPolicyGuard([]FolderGrant{
		{ID: "ro", RootPath: dir, Mode: AccessReadOnly},
		{ID: "rw", RootPath: dir, Mode: AccessReadWrite},
		{ID: "rwd", RootPath: dir, Mode: AccessReadWriteDelete},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := guard.ValidateStageRequest(DeviceStageRequest{
		GrantID: "ro",
		RunID:   "run-1",
		Files:   []StageFileTarget{{RelPath: "calc.xlsx", UploadURL: "https://storage.googleapis.com/bucket/key"}},
	}); err != nil {
		t.Fatalf("valid stage request failed: %v", err)
	}
	if _, err := guard.ValidateStageRequest(DeviceStageRequest{
		GrantID: "ro",
		RunID:   "run-1",
		Files:   []StageFileTarget{{RelPath: "calc.xlsx", UploadURL: "http://insecure.example/key"}},
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("http upload URL err = %v, want ErrInvalidRequest", err)
	}

	if _, err := guard.ValidateCommitRequest(DeviceCommitRequest{
		GrantID: "rw",
		RunID:   "run-1",
		Files:   []CommitFileEntry{{RelPath: "calc.xlsx", DownloadURL: "https://storage.googleapis.com/bucket/out"}},
	}); err != nil {
		t.Fatalf("valid rw commit failed: %v", err)
	}
	if _, err := guard.ValidateCommitRequest(DeviceCommitRequest{
		GrantID: "rw",
		RunID:   "run-1",
		Files:   []CommitFileEntry{{RelPath: "calc.xlsx", Delete: true}},
	}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("rw delete commit err = %v, want ErrAccessDenied", err)
	}
	if _, err := guard.ValidateCommitRequest(DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "run-1",
		Files:   []CommitFileEntry{{RelPath: "calc.xlsx", Delete: true}},
	}); err != nil {
		t.Fatalf("rwd delete commit failed: %v", err)
	}

	if err := CheckCommitPrecondition(target, &mtime, digest); err != nil {
		t.Fatalf("matching precondition failed: %v", err)
	}
	wrongMtime := mtime - 1000
	if err := CheckCommitPrecondition(target, &wrongMtime, digest); !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched mtime err = %v, want ErrConflict", err)
	}
	if err := CheckCommitPrecondition(target, &mtime, "0000000000000000"); !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched sha256 err = %v, want ErrConflict", err)
	}
	if err := CheckCommitPrecondition(filepath.Join(dir, "missing.xlsx"), &mtime, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing file precondition err = %v, want ErrConflict", err)
	}
}
