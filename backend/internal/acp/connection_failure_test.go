//go:build !windows

package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
)

func TestTerminatedAgentReportsExitAndContinuesItsProviderSession(t *testing.T) {
	dir := t.TempDir()
	starts := filepath.Join(dir, "starts")
	requests := filepath.Join(dir, "requests")
	manager := idleManager(t, "fake", map[string]string{
		"JAZ_FAKE_ACP_RESUME":       "1",
		"JAZ_FAKE_ACP_START_LOG":    starts,
		"JAZ_FAKE_ACP_REQUEST_LOG":  requests,
		"JAZ_FAKE_ACP_RELEASE_FILE": filepath.Join(dir, "release"),
	}, "", "")
	ctx, id := startIdleThread(t, manager, "fake")
	before, err := manager.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{Session: id, Message: "until released", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	for requestsSent(t, requests, "session/prompt") < 2 {
		pause(ctx, t)
	}
	if err := syscall.Kill(startedPIDs(t, starts)[0], syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	failed, err := manager.Wait(ctx, acp.WaitRequest{Session: id, Timeout: 10 * time.Second})
	if err != nil || failed.State != acp.StateFailed || !strings.Contains(failed.Error, "Continue to resume") || !strings.Contains(failed.Error, "signal: terminated") {
		t.Fatalf("terminated turn = %#v, %v", failed, err)
	}
	if got := requestsSent(t, requests, "session/prompt"); got != 2 {
		t.Fatalf("prompt count after termination = %d, want no automatic replay", got)
	}
	if err := turn(ctx, manager, id, "Continue"); err != nil {
		t.Fatal(err)
	}
	if processStarts(t, starts) != 2 || requestsSent(t, requests, "session/new") != 1 || requestsSent(t, requests, "session/prompt") != 3 {
		t.Fatal("continue did not replace the dead process and send exactly one new prompt")
	}
	data, err := os.ReadFile(requests)
	if err != nil {
		t.Fatal(err)
	}
	resumed := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var request struct {
			Method string `json:"method"`
			Params struct {
				SessionID string `json:"sessionId"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatal(err)
		}
		if request.Method == "session/resume" {
			resumed++
			if request.Params.SessionID != before.ACPSession {
				t.Fatalf("resumed session = %q, want %q", request.Params.SessionID, before.ACPSession)
			}
		}
	}
	if resumed != 1 {
		t.Fatalf("resume count = %d, want 1", resumed)
	}
}
