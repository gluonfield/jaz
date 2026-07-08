package acp

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeAgy writes an executable named agy that records each invocation and exits
// with the given code, and returns the probe env pointing PATH/HOME at its dir.
func fakeAgy(t *testing.T, exitCode int) (map[string]string, string) {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	agy := filepath.Join(dir, "agy")
	body := "#!/bin/sh\nprintf x >> " + counter + "\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(agy, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	invalidateAntigravityAuthCache()
	t.Cleanup(invalidateAntigravityAuthCache)
	return map[string]string{"PATH": dir, "HOME": dir}, counter
}

func agyRuns(t *testing.T, counter string) int {
	t.Helper()
	runs, err := os.ReadFile(counter)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read counter: %v", err)
	}
	return len(strings.TrimSpace(string(runs)))
}

// Settings load/save probes antigravity twice per probe (env build + resolve);
// each authenticated probe would otherwise spawn a ~1s `agy models`, so the
// second must hit the cache.
func TestAntigravityCLIAuthenticatedCachesSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable unsupported on windows")
	}
	env, counter := fakeAgy(t, 0)
	if !antigravityCLIAuthenticated(env) || !antigravityCLIAuthenticated(env) {
		t.Fatal("expected authenticated on both probes")
	}
	if got := agyRuns(t, counter); got != 1 {
		t.Fatalf("agy ran %d times, want 1 (second probe should hit cache)", got)
	}
}

// A signed-out result must never be cached: the login flow verifies auth right
// after writing the credential, and a cached "no" would report a false failure.
func TestAntigravityCLIAuthenticatedDoesNotCacheFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable unsupported on windows")
	}
	env, counter := fakeAgy(t, 1)
	if antigravityCLIAuthenticated(env) || antigravityCLIAuthenticated(env) {
		t.Fatal("expected not authenticated on both probes")
	}
	if got := agyRuns(t, counter); got != 2 {
		t.Fatalf("agy ran %d times, want 2 (failures must stay fresh)", got)
	}
}
