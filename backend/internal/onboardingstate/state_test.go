package onboardingstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	state, found, err := Load(filepath.Join(t.TempDir(), "onboarding.json"))
	if err != nil {
		t.Fatal(err)
	}
	if found || state.Completed {
		t.Fatalf("state = %#v, found = %v", state, found)
	}
}

func TestSaveLoad(t *testing.T) {
	path := Path(t.TempDir())
	if err := Save(path, State{Completed: true}); err != nil {
		t.Fatal(err)
	}
	state, found, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !state.Completed {
		t.Fatalf("state = %#v, found = %v", state, found)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %v", info.Mode().Perm())
	}
}
