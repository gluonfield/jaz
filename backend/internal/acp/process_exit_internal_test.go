//go:build !windows

package acp

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentExitReasonIsAvailableWhileDescendantsStop(t *testing.T) {
	dir := t.TempDir()
	release := filepath.Join(dir, "release")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	manager := NewManager(nil, Config{}, nil)
	conn, stderr, err := manager.openConn(ctx, "fake", AgentConfig{
		Command: "/bin/sh",
		Args: []string{"-c", `(
trap '' TERM
: > "$1/ready"
while [ ! -e "$1/release" ]
do
    sleep 0.02
done
) </dev/null >/dev/null 2>&1 &
while [ ! -e "$1/ready" ]
do
    sleep 0.02
done
exit 7`, "agent", dir},
	}, map[string]string{"PATH": os.Getenv("PATH")}, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(release, nil, 0o600)
		_ = conn.Close()
		select {
		case <-stderr.done:
		case <-ctx.Done():
			t.Error("agent cleanup did not finish")
		}
	})
	_, err = conn.Receive(ctx)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("receive = %v, want EOF", err)
	}
	select {
	case <-stderr.done:
		t.Fatal("descendant was not held through cleanup")
	default:
	}
	if err := withProcessStderr(err, stderr); !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("connection failure lost the exit reason: %v", err)
	}
}
