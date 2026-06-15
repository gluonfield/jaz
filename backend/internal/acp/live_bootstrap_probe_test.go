//go:build acpprobe && !windows

package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestLiveACPManagerBootstrapProbe(t *testing.T) {
	agent := CanonicalAgentName(firstNonEmpty(os.Getenv("ACP_PROBE_AGENT"), AgentCodex))
	timeout := 60 * time.Second
	if raw := os.Getenv("ACP_PROBE_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := firstNonEmpty(os.Getenv("ACP_PROBE_ROOT"), filepath.Join(os.Getenv("HOME"), ".jaz"))
	base := filepath.Join(root, "tmp", "acp-manager-bootstrap-probe", strings.ReplaceAll(agent, "/", "-"))
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := jsonstore.New(filepath.Join(base, fmt.Sprintf("store-%d", time.Now().UnixNano())))
	if err != nil {
		t.Fatal(err)
	}

	cfg := probeAgentConfig(t, agent)
	manager := NewManager(store, Config{
		Root:      root,
		Workspace: workspace,
		Agents:    map[string]AgentConfig{agent: cfg},
	}, log.New(io.Discard))
	defer manager.Close()

	slug := fmt.Sprintf("bootstrap-%d", time.Now().UnixNano())
	totalStarted := time.Now()
	createStarted := time.Now()
	session, err := manager.CreateSession(ctx, SpawnRequest{ACPAgent: agent, Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	createDuration := time.Since(createStarted)

	resumeStarted := time.Now()
	job, err := manager.resume(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	resumeDuration := time.Since(resumeStarted)
	if job.ACPSession == "" {
		t.Fatal("empty ACP session id after resume")
	}
	t.Logf("%s manager bootstrap timings: create_session=%s resume_connect_initialize_auth_session_config=%s total=%s acp_session=%s",
		agent, createDuration, resumeDuration, time.Since(totalStarted), job.ACPSession)
}
