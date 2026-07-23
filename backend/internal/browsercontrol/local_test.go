package browsercontrol

import (
	"context"
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
	if _, err := normalizeBrowserURL("about:blank"); err == nil {
		t.Fatal("about URL should be rejected")
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
	if !strings.Contains(script, "jazDeepHit") || !strings.Contains(script, "target is obscured") {
		t.Fatalf("click resolver does not fail closed on an obscured target: %s", script)
	}
	state := semanticStateScript()
	if !strings.Contains(state, "page_revision: revision") ||
		!strings.Contains(state, "jazStateElement") ||
		!strings.Contains(state, "attributeFilter") ||
		!strings.Contains(state, "jazNewRefRevision()") ||
		!strings.Contains(state, `password ? "" : el.value`) {
		t.Fatalf("state script missing typed ref metadata: %s", state)
	}
	find := findElementsScript("annual turnover")
	if !strings.Contains(find, ".filter(jazRendered)") || !strings.Contains(find, "Matches for") {
		t.Fatalf("find script missing whole-page semantic matching: %s", find)
	}
	form := formInputScript("p1:e2", 12)
	if !strings.Contains(form, "HTMLInputElement.prototype") || !strings.Contains(form, "InputEvent") {
		t.Fatalf("form script missing native form semantics: %s", form)
	}
}

func TestManagedTabOwnershipIsSessionIsolated(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	backend.pages = map[string]*browserPage{
		"session-1": {target: targetInfo{ID: "tab-1"}},
		"session-2": {target: targetInfo{ID: "tab-2"}},
	}
	got := backend.ownershipByTarget("session-1")
	if got["tab-1"] != "current_session" || got["tab-2"] != "other_session" {
		t.Fatalf("ownership = %#v", got)
	}
	if !backend.claimedByOtherSession("session-1", "tab-2") {
		t.Fatal("cross-session claim was not detected")
	}
	if backend.claimedByOtherSession("session-1", "tab-1") {
		t.Fatal("current session should be allowed to reclaim its own tab")
	}
}

func TestManagedReadRequiresExistingSessionTab(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	_, err := backend.Call(context.Background(), ActionInput{Action: ActionState, Session: "session-1"})
	if err == nil || !strings.Contains(err.Error(), "call browser_navigate") {
		t.Fatalf("err = %v", err)
	}
	if backend.browser != nil || len(backend.pages) != 0 {
		t.Fatalf("read-only state launched browser: browser=%#v pages=%#v", backend.browser, backend.pages)
	}
}

func TestManagedBackendRejectsUnknownActionBeforeOpeningTab(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	_, err := backend.Call(context.Background(), ActionInput{Action: "unknown"})
	if !IsUnsupportedAction(err, "unknown") {
		t.Fatalf("err = %v", err)
	}
	if backend.browser != nil || len(backend.pages) != 0 {
		t.Fatalf("unknown action launched browser: browser=%#v pages=%#v", backend.browser, backend.pages)
	}
}

func TestManagedBackendRejectsInvalidNavigationBeforeOpeningTab(t *testing.T) {
	backend := NewLocalBackend(t.TempDir())
	_, err := backend.Call(context.Background(), ActionInput{Action: ActionNavigate, URL: "javascript:alert(1)"})
	if err == nil {
		t.Fatal("invalid navigation was accepted")
	}
	if backend.browser != nil || len(backend.pages) != 0 {
		t.Fatalf("invalid navigation launched browser: browser=%#v pages=%#v", backend.browser, backend.pages)
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
