package browserworker

import (
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeBrowserURL(t *testing.T) {
	got, err := normalizeBrowserURL("example.com/path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/path" {
		t.Fatalf("url = %q", got)
	}
	if _, err := normalizeBrowserURL("file:///tmp/secret"); err == nil {
		t.Fatal("file URL should be rejected")
	}
	if got, err := normalizeBrowserURL("about:blank"); err != nil || got != "about:blank" {
		t.Fatalf("about URL = %q, %v", got, err)
	}
}

func TestScrollDelta(t *testing.T) {
	dy, dx := scrollDelta("up", 120)
	if dy != -120 || dx != 0 {
		t.Fatalf("up = %d, %d", dy, dx)
	}
	dy, dx = scrollDelta("right", 0)
	if dy != 0 || dx != defaultScrollAmount {
		t.Fatalf("right default = %d, %d", dy, dx)
	}
}

func TestKeyEventCopiesAreIndependent(t *testing.T) {
	event, err := keyEvent("Enter")
	if err != nil {
		t.Fatal(err)
	}
	down := copyMap(event)
	up := copyMap(event)
	down["type"] = "keyDown"
	up["type"] = "keyUp"
	if down["type"] != "keyDown" || up["type"] != "keyUp" {
		t.Fatalf("events share state: down=%#v up=%#v", down, up)
	}
}

func TestElementScriptsReturnActionCompletion(t *testing.T) {
	script := resolvePointScript("button")
	if strings.Index(script, "function jazFindElement") > strings.Index(script, "(function(q)") {
		t.Fatalf("resolver helpers must be defined before action IIFE: %s", script)
	}
}

func TestChromeArgsUseHeadlessOnLinuxWithoutDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	args := strings.Join(chromeArgs("/tmp/profile"), " ")
	if !strings.Contains(args, "--user-data-dir=/tmp/profile") {
		t.Fatalf("args = %s", args)
	}
	if runtime.GOOS == "linux" && !strings.Contains(args, "--headless=new") {
		t.Fatalf("linux without display should use headless args: %s", args)
	}
	if runtime.GOOS != "linux" && strings.Contains(args, "--headless=new") {
		t.Fatalf("non-linux desktop should not be forced headless: %s", args)
	}
}
