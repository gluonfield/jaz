//go:build !windows

package acp_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

type catalogRevision struct{ atomic.Uint64 }

func (r *catalogRevision) Revision() uint64 { return r.Load() }

func TestChangedCatalogRelaunchesOnlyAnIdleCodexProcess(t *testing.T) {
	type thread struct {
		revision       *catalogRevision
		send           func(string)
		launches       func() (starts, resumed int)
		deferDiscovery func()
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
			deferDiscovery: func() {
				t.Helper()
				if err := manager.SetDeferredMCPTools(ctx, id, true); err != nil {
					t.Fatal(err)
				}
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

	t.Run("codex with deferred discovery", func(t *testing.T) {
		codex := start(t, acp.AgentCodex)
		codex.deferDiscovery()
		codex.send("deferred")
		codex.revision.Add(1)
		codex.send("changed catalog")
		if starts, _ := codex.launches(); starts != 2 {
			t.Fatalf("process starts = %d, want only the relaunch that enabled deferred discovery", starts)
		}
	})

	t.Run("other agent", func(t *testing.T) {
		other := start(t, "fake")
		other.revision.Add(1)
		other.send("changed catalog")
		if starts, _ := other.launches(); starts != 1 {
			t.Fatalf("process starts = %d, want an agent without the Codex limitation left running", starts)
		}
	})
}
