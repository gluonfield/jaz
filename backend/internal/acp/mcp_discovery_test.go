package acp

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestDeferredMCPToolsRetiresOnlyIdleTargetAndKeepsNativeSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{Root: t.TempDir(), Workspace: t.TempDir(), Agents: map[string]AgentConfig{"fake": {
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeACPAgentProcess"},
		Env: map[string]string{
			"JAZ_FAKE_ACP_AGENT":          "1",
			"JAZ_FAKE_ACP_RESUME":         "1",
			"JAZ_FAKE_ACP_UNIQUE_SESSION": "1",
			"JAZ_FAKE_ACP_SESSIONS":       t.TempDir(),
		},
	}}}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	target, err := manager.Spawn(ctx, SpawnRequest{ACPAgent: "fake", Slug: "target"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := manager.Spawn(ctx, SpawnRequest{ACPAgent: "fake", Slug: "other"})
	if err != nil {
		t.Fatal(err)
	}
	job := manager.jobByID(target.SessionID)
	sibling := manager.jobByID(other.SessionID)
	session, err := store.LoadSession(target.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	native := session.RuntimeRef.SessionID
	if !job.lease() {
		t.Fatal("could not lease idle process")
	}
	if err := manager.SetDeferredMCPTools(ctx, target.SessionID, false); err == nil {
		t.Fatal("retired a process another caller was using")
	}
	job.release()
	unchanged, err := store.LoadSession(target.SessionID)
	if err != nil || unchanged.RuntimeRef.DeferredMCPTools != nil {
		t.Fatalf("busy selection persisted: %#v, %v", unchanged.RuntimeRef, err)
	}
	if manager.jobByID(target.SessionID) != job {
		t.Fatal("refused selection stopped the target")
	}
	recording, recorded := make(chan struct{}), make(chan struct{})
	record := sync.OnceFunc(func() { close(recorded) })
	t.Cleanup(record)
	var hold sync.Once
	manager.TurnFinished = func(context.Context, Job) {
		hold.Do(func() {
			close(recording)
			<-recorded
		})
	}
	if _, err := manager.Send(ctx, SendRequest{Session: target.SessionID, Message: "ending", Completion: CompletionInline}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recording:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	changed := make(chan error, 1)
	go func() {
		changed <- manager.SetDeferredMCPTools(ctx, target.Slug, false)
	}()
	select {
	case err := <-changed:
		t.Fatalf("selection did not wait for the ended turn to be recorded: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	record()
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if manager.jobByID(target.SessionID) != nil || manager.jobByID(other.SessionID) != sibling {
		t.Fatal("selection did not retire only the target")
	}
	session, err = store.LoadSession(target.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if choice := session.RuntimeRef.DeferredMCPTools; choice == nil || *choice || session.RuntimeRef.SessionID != native {
		t.Fatalf("selection replaced native session: %#v", session.RuntimeRef)
	}
	if _, err := manager.Send(ctx, SendRequest{Session: target.SessionID, Message: "hello", Completion: CompletionInline}); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Wait(ctx, WaitRequest{Session: target.SessionID, Timeout: 10 * time.Second})
	if err != nil || result.State != StateIdle || result.ACPSession != native {
		t.Fatalf("native session was not resumed: %#v, %v", result, err)
	}
	if _, err := manager.Send(ctx, SendRequest{Session: other.SessionID, Message: "hello", Completion: CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Wait(ctx, WaitRequest{Session: other.SessionID, Timeout: 10 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if manager.jobByID(other.SessionID) != sibling {
		t.Fatal("changing the target forced the sibling to restart")
	}
}
