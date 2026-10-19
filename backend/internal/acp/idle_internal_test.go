//go:build !windows

package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// A send can find a job after the stopper retired it but before its process
// is stopped. The send must resume the session rather than start a turn on the
// process about to go.
func TestSendThatFindsARetiredJobResumesTheSession(t *testing.T) {
	release := filepath.Join(t.TempDir(), "release")
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{Root: t.TempDir(), Workspace: t.TempDir(), Agents: map[string]AgentConfig{"fake": {
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeACPAgentProcess"},
		Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1", "JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_RELEASE_FILE": release},
	}}}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, SpawnRequest{ACPAgent: "fake", Slug: "retired"})
	if err != nil {
		t.Fatal(err)
	}
	retired := manager.jobByID(spawned.SessionID)
	if !retired.retire() {
		t.Fatal("idle job was not retired")
	}
	if _, err := manager.Send(ctx, SendRequest{Session: spawned.SessionID, Message: "until released", Completion: CompletionInline}); err != nil {
		t.Fatal(err)
	}
	manager.stopRetired(retired)
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		status, err := manager.Status(spawned.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == StateIdle {
			return
		}
		if status.State != StateRunning && status.State != StateStarting {
			t.Fatalf("turn ended %s: %s", status.State, status.Error)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestIdleStopLeavesURLAgentsConnected(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	ids := map[bool]string{}
	for _, launched := range []bool{false, true} {
		session, err := store.CreateSession(storage.CreateSession{Slug: fmt.Sprint("launched-", launched), Runtime: storage.RuntimeACP})
		if err != nil {
			t.Fatal(err)
		}
		local, _ := jsonrpc.Pipe(1)
		conn := newPromptTrackingConn(local)
		ac := &agentConn{peer: jsonrpc.NewPeer(conn, nil), cancel: func() {}, promptTracker: conn}
		if launched {
			ac.stderr = newProcessStderrTail()
			ac.stderr.close(nil)
		}
		manager.addJob(newIdleJob(session, "fake", session.ID, "", ModeState{}), newAgentProcess(ac))
		ids[launched] = session.ID
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	stopped := make(chan struct{})
	go func() {
		manager.StopIdleProcesses(ctx, IdleTimeouts{Thread: time.Millisecond})
		close(stopped)
	}()
	defer func() {
		cancel()
		<-stopped
	}()
	for manager.jobByID(ids[true]) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("the idle launched process was not stopped")
		case <-time.After(time.Millisecond):
		}
	}
	if manager.jobByID(ids[false]) == nil {
		t.Fatal("stopped an idle URL agent, which may not be able to restore its session")
	}
}
