//go:build !windows

package acp_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestShutdownReapsAnAgentStillStarting(t *testing.T) {
	for _, entry := range []string{"spawn", "resume", "thread", "utility"} {
		t.Run(entry, func(t *testing.T) {
			jaz := startingJaz(t, map[string]string{})
			dir, _ := held(t)
			req := acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "held", Directory: dir}
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				switch entry {
				case "spawn":
					_, _ = jaz.manager.Spawn(jaz.ctx, req)
				case "resume":
					session, err := jaz.manager.CreateSession(jaz.ctx, req)
					if err == nil {
						_, _ = jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: session.ID, Message: "hello"})
					}
				case "thread":
					_, _ = jaz.manager.Start(jaz.ctx, req, storage.NewInternalQueuedMessage("hello"))
				case "utility":
					_, _ = jaz.manager.RunUtilityPrompt(jaz.ctx, acp.UtilityPromptRequest{ACPAgent: acp.AgentCodex, Directory: dir, Message: "title"})
				}
			}()
			jaz.eventually("the held process starting", func() bool { return jaz.starts() == 1 })
			raw, err := os.ReadFile(filepath.Join(jaz.root, "starts"))
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			reaped := false
			t.Cleanup(func() {
				if !reaped {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			})
			jaz.manager.Close()
			if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
				t.Errorf("Close returned with starting process %d still alive: %v", pid, err)
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			reaped = true
			select {
			case <-finished:
			case <-jaz.ctx.Done():
				t.Fatal("the launch did not finish after shutdown")
			}
		})
	}
}
