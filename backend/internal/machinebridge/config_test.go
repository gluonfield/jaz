package machinebridge

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDefaultShareConfig(t *testing.T) {
	ws := t.TempDir()
	cfg := DefaultShareConfig(ws, "jaz-test")
	if !cfg.Enabled {
		t.Fatal("expected Enabled default true")
	}
	if cfg.AllowExec {
		t.Fatal("expected AllowExec default false")
	}
	if cfg.DisplayName == "" {
		t.Fatal("expected non-empty default DisplayName")
	}
	if len(cfg.Grants) != 1 || cfg.Grants[0].ID != "workspace" || cfg.Grants[0].Mode != AccessReadWrite || cfg.Grants[0].RootPath != filepath.Clean(ws) {
		t.Fatalf("unexpected default grants: %+v", cfg.Grants)
	}

	missing := DefaultShareConfig(filepath.Join(ws, "does-not-exist"), "jaz-test")
	if missing.Grants == nil || len(missing.Grants) != 0 {
		t.Fatalf("expected non-nil empty grants for missing workspace, got %#v", missing.Grants)
	}
}

func TestNormalizeShareConfigValidationAndAutoIDs(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "My Repo")
	dirB := filepath.Join(root, "other", "My Repo")
	dirC := filepath.Join(root, "explicit")
	filePath := filepath.Join(root, "file.txt")
	for _, d := range []string{dirA, dirB, dirC} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filePath, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Auto-derived IDs avoid colliding with both explicit IDs and earlier derived IDs.
	norm, err := NormalizeShareConfig(ShareConfig{
		Enabled:     true,
		DisplayName: "  Studio Mac  ",
		AllowExec:   true,
		Grants: []FolderGrant{
			{RootPath: dirA + string(os.PathSeparator), Mode: AccessReadOnly},
			{RootPath: dirB, Mode: AccessReadWrite, Label: "  Secondary  "},
			{ID: "my-repo-2", RootPath: dirC, Mode: AccessReadWriteDelete},
		},
	}, "fallback")
	if err != nil {
		t.Fatalf("NormalizeShareConfig: %v", err)
	}
	if norm.DisplayName != "Studio Mac" {
		t.Fatalf("DisplayName = %q, want Studio Mac", norm.DisplayName)
	}
	if len(norm.Grants) != 3 {
		t.Fatalf("len(Grants) = %d, want 3", len(norm.Grants))
	}
	if norm.Grants[0].ID != "my-repo" || norm.Grants[0].RootPath != filepath.Clean(dirA) {
		t.Fatalf("grant[0] = %+v", norm.Grants[0])
	}
	if norm.Grants[1].ID != "my-repo-3" || norm.Grants[1].Label != "Secondary" {
		t.Fatalf("grant[1] = %+v", norm.Grants[1])
	}
	if norm.Grants[2].ID != "my-repo-2" {
		t.Fatalf("grant[2] = %+v", norm.Grants[2])
	}

	// Explicit empty grants stays non-nil empty.
	empty, err := NormalizeShareConfig(ShareConfig{Enabled: false}, "fallback")
	if err != nil {
		t.Fatalf("empty NormalizeShareConfig: %v", err)
	}
	if empty.Grants == nil || len(empty.Grants) != 0 {
		t.Fatalf("expected non-nil empty grants, got %#v", empty.Grants)
	}

	cases := []struct {
		name string
		cfg  ShareConfig
	}{
		{
			name: "control char in display name",
			cfg:  ShareConfig{DisplayName: "Mac\nBook"},
		},
		{
			name: "relative path",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "rel", RootPath: "relative/dir", Mode: AccessReadOnly}}},
		},
		{
			name: "missing path",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "missing", RootPath: filepath.Join(root, "nope"), Mode: AccessReadOnly}}},
		},
		{
			name: "file instead of directory",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "file", RootPath: filePath, Mode: AccessReadOnly}}},
		},
		{
			name: "invalid mode",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "badmode", RootPath: dirA, Mode: "admin"}}},
		},
		{
			name: "equals in grant id",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "a=b", RootPath: dirA, Mode: AccessReadOnly}}},
		},
		{
			name: "colon in grant id",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "a:b", RootPath: dirA, Mode: AccessReadOnly}}},
		},
		{
			name: "control char in grant id",
			cfg:  ShareConfig{Grants: []FolderGrant{{ID: "a\tb", RootPath: dirA, Mode: AccessReadOnly}}},
		},
		{
			name: "duplicate grant id",
			cfg: ShareConfig{Grants: []FolderGrant{
				{ID: "dup", RootPath: dirA, Mode: AccessReadOnly},
				{ID: "dup", RootPath: dirB, Mode: AccessReadWrite},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeShareConfig(tc.cfg, "fallback"); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("expected ErrInvalidRequest, got %v", err)
			}
		})
	}

	// Exceeding MaxFolderGrants is rejected.
	tooMany := make([]FolderGrant, MaxFolderGrants+1)
	for i := range tooMany {
		tooMany[i] = FolderGrant{ID: "g" + strconv.Itoa(i), RootPath: dirA, Mode: AccessReadOnly}
	}
	if _, err := NormalizeShareConfig(ShareConfig{Grants: tooMany}, "fallback"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for >MaxFolderGrants, got %v", err)
	}
}

func TestShareConfigHashSensitivity(t *testing.T) {
	base := ShareConfig{
		Enabled:     true,
		DisplayName: "Studio",
		AllowExec:   false,
		Grants: []FolderGrant{
			{ID: "a", RootPath: "/tmp/a", Mode: AccessReadOnly},
			{ID: "b", RootPath: "/tmp/b", Mode: AccessReadWrite},
		},
	}
	reordered := ShareConfig{
		Enabled:     true,
		DisplayName: "Studio",
		AllowExec:   false,
		Grants: []FolderGrant{
			{ID: "b", RootPath: "/tmp/b", Mode: AccessReadWrite},
			{ID: "a", RootPath: "/tmp/a", Mode: AccessReadOnly},
		},
	}
	if base.Hash() != reordered.Hash() {
		t.Fatal("expected grant ordering alone not to change hash")
	}

	withExec := base.Clone()
	withExec.AllowExec = true
	if base.Hash() == withExec.Hash() {
		t.Fatal("expected AllowExec toggle to change hash")
	}

	withMode := base.Clone()
	withMode.Grants[0].Mode = AccessReadWriteDelete
	if base.Hash() == withMode.Hash() {
		t.Fatal("expected grant mode change to change hash")
	}

	withName := base.Clone()
	withName.DisplayName = "Studio 2"
	if base.Hash() == withName.Hash() {
		t.Fatal("expected DisplayName change to change hash")
	}
}
