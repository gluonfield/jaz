package terminal

import (
	"fmt"
	"runtime"
	"testing"
)

func TestUnsubscribeDoesNotRaceOutput(t *testing.T) {
	session := newSession(t.TempDir(), 1024, nil)
	for i := range 1000 {
		_, unsubscribe := session.Subscribe()
		done := make(chan struct{})
		go func() {
			unsubscribe()
			close(done)
		}()
		session.output([]byte(fmt.Sprintf("line %d\n", i)))
		<-done
	}
}

func TestManagerReusesSameCwdAndRestartsChangedCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("terminal sessions are not supported on Windows yet")
	}
	manager := New()
	defer manager.Close()
	firstDir := t.TempDir()
	secondDir := t.TempDir()

	first, err := manager.Open("session", firstDir+"/", Size{})
	if err != nil {
		t.Fatal(err)
	}
	reused, err := manager.Open("session", firstDir, Size{})
	if err != nil {
		t.Fatal(err)
	}
	if reused != first {
		t.Fatal("same cwd should reuse the terminal")
	}

	second, err := manager.Open("session", secondDir, Size{})
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("changed cwd should restart the terminal")
	}
	if second.Cwd() != secondDir {
		t.Fatalf("cwd = %q, want %q", second.Cwd(), secondDir)
	}
}
