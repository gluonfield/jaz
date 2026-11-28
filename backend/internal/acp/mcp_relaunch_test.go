//go:build !windows

package acp_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

type catalogRevision struct {
	atomic.Uint64
	listed atomic.Uint64
	unread atomic.Bool
}

func (r *catalogRevision) Revision() uint64 { return r.Load() }

func (r *catalogRevision) ListedRevision(string) (uint64, bool) {
	return r.listed.Load(), !r.unread.Load()
}

func TestChangedCatalogRelaunchesOnlyAnIdleProcessThatMissedIt(t *testing.T) {
	type thread struct {
		revision       *catalogRevision
		send           func(string)
		launches       func() (starts, resumed int)
		deferDiscovery func() error
	}
	start := func(t *testing.T, agent string) thread {
		logs := t.TempDir()
		startLog, requestLog := filepath.Join(logs, "starts"), filepath.Join(logs, "requests")
		env := map[string]string{"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_START_LOG": startLog, "JAZ_FAKE_ACP_REQUEST_LOG": requestLog}
		manager := idleManager(t, agent, env, "", "")
		revision := &catalogRevision{}
		manager.SetMCPRevisionSource(revision)
		ctx, id := startIdleThread(t, manager, agent)
		return thread{
			revision: revision,
			send: func(message string) {
				t.Helper()
				if err := turn(ctx, manager, id, message); err != nil {
					t.Fatal(err)
				}
			},
			launches: func() (int, int) {
				return processStarts(t, startLog), requestsSent(t, requestLog, "session/resume")
			},
			deferDiscovery: func() error {
				return manager.SetDeferredMCPTools(ctx, id, true)
			},
		}
	}

	t.Run("codex", func(t *testing.T) {
		codex := start(t, acp.AgentCodex)
		codex.send("same catalog")
		if starts, _ := codex.launches(); starts != 1 {
			t.Fatalf("process starts = %d, want the process kept while the catalog is unchanged", starts)
		}
		codex.revision.Add(1)
		codex.send("task build=running")
		if starts, resumed := codex.launches(); starts != 2 || resumed != 1 {
			t.Fatalf("process starts = %d, session/resume = %d; want the thread resumed in a new process", starts, resumed)
		}
		codex.revision.Add(1)
		codex.send("task build=completed")
		if starts, _ := codex.launches(); starts != 2 {
			t.Fatalf("process starts = %d, want the process kept while its background task runs", starts)
		}
		codex.send("after the task")
		if starts, resumed := codex.launches(); starts != 3 || resumed != 2 {
			t.Fatalf("process starts = %d, session/resume = %d; want the relaunch once the task ended", starts, resumed)
		}
	})

	t.Run("codex keeps native discovery", func(t *testing.T) {
		codex := start(t, acp.AgentCodex)
		if err := codex.deferDiscovery(); err == nil {
			t.Fatal("gateway opt-in replaced native tool names")
		}
		if starts, _ := codex.launches(); starts != 1 {
			t.Fatalf("rejected gateway opt-in restarted the agent: %d starts", starts)
		}
		codex.revision.Add(1)
		codex.send("changed catalog")
		if starts, resumed := codex.launches(); starts != 2 || resumed != 1 {
			t.Fatalf("native catalogue change did not resume: %d starts, %d resumes", starts, resumed)
		}
	})

	t.Run("other agent", func(t *testing.T) {
		other := start(t, "fake")
		other.revision.Add(1)
		other.revision.listed.Store(1)
		other.send("catalog its MCP client reread")
		if starts, _ := other.launches(); starts != 1 {
			t.Fatalf("process starts = %d, want the process kept once it read the changed catalog", starts)
		}
		other.revision.Add(1)
		other.send("catalog change it missed")
		if starts, resumed := other.launches(); starts != 2 || resumed != 1 {
			t.Fatalf("process starts = %d, session/resume = %d; want the thread resumed to read the missed catalog", starts, resumed)
		}
	})

	t.Run("agent that never read the catalog", func(t *testing.T) {
		other := start(t, "fake")
		other.revision.unread.Store(true)
		other.revision.Add(1)
		other.send("changed catalog")
		if starts, _ := other.launches(); starts != 1 {
			t.Fatalf("process starts = %d, want an agent that does not use the catalog left running", starts)
		}
	})
}
