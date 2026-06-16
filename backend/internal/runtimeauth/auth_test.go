package runtimeauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesAndReusesKey(t *testing.T) {
	root := t.TempDir()
	key, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if key == "" {
		t.Fatal("key is empty")
	}
	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth file mode = %v", info.Mode().Perm())
	}
	again, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if again != key {
		t.Fatalf("Ensure rotated key: %q != %q", again, key)
	}
}

func TestEnsureUsesExistingKey(t *testing.T) {
	root := t.TempDir()
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"api_key":"existing"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if key != "existing" {
		t.Fatalf("key = %q", key)
	}
}
