package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestOpenCodeSessionsKeepTheirOwnInstructionsAcrossRelaunch(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	factory := func() *acp.Manager {
		manager := acp.NewManager(store, acp.Config{
			Root:         root,
			Workspace:    root,
			SystemPrompt: cwdPrompt{},
			ResumePrompt: func(session storage.Session) (promptmodule.Modules, error) {
				return promptmodule.New("resumed " + session.ID), nil
			},
			Agents: map[string]acp.AgentConfig{
				acp.AgentOpenCode: {
					Command: os.Args[0],
					Args:    []string{"-test.run=TestFakeACPAgentProcess"},
					Cwd:     filepath.Join(root, "wrong"),
					Env: map[string]string{
						"JAZ_FAKE_ACP_AGENT":                   "1",
						"JAZ_FAKE_ACP_UNIQUE_SESSION":          "cwd",
						"JAZ_FAKE_ACP_RESUME":                  "1",
						"JAZ_FAKE_ACP_OPENCODE_CONFIG_CAPTURE": "config-capture.json",
					},
				},
			},
		}, log.New(io.Discard))
		t.Cleanup(manager.Close)
		return manager
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	managers := []*acp.Manager{factory(), factory()}
	sessions := make([]acp.SpawnResult, 2)
	errors := make([]error, 2)
	var starts sync.WaitGroup
	for i := range managers {
		starts.Go(func() {
			sessions[i], errors[i] = managers[i].Spawn(ctx, acp.SpawnRequest{
				ACPAgent:               acp.AgentOpenCode,
				Slug:                   "separate-instructions",
				SystemPromptExtensions: promptmodule.New("initial session context"),
			})
		})
	}
	starts.Wait()
	paths := make([]string, 2)
	for i, session := range sessions {
		if errors[i] != nil {
			t.Fatal(errors[i])
		}
		paths[i] = readOpenCodeInstructions(t, session.Cwd, "cwd: "+session.Cwd+"\n\ninitial session context\n")
	}
	for i, session := range sessions {
		managers[i].Close()
		if _, err := os.Stat(paths[i]); !os.IsNotExist(err) {
			t.Fatalf("closed process kept its instructions: %v", err)
		}
		if i == 0 {
			readOpenCodeInstructions(t, sessions[1].Cwd, "cwd: "+sessions[1].Cwd+"\n\ninitial session context\n")
		}
		managers[i] = factory()
		if _, err := managers[i].Send(ctx, acp.SendRequest{Session: session.SessionID, Message: "hello", Completion: acp.CompletionInline}); err != nil {
			t.Fatal(err)
		}
		job, err := managers[i].Wait(ctx, acp.WaitRequest{Session: session.SessionID, Timeout: 5 * time.Second})
		if err != nil || job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
			t.Fatalf("resumed OpenCode turn = %+v, %v", job, err)
		}
		current := readOpenCodeInstructions(t, session.Cwd, "cwd: "+session.Cwd+"\n\nresumed "+session.SessionID+"\n")
		if current == paths[i] {
			t.Fatal("relaunched process reused the retired instruction file")
		}
	}
	restricted, err := managers[0].Spawn(ctx, acp.SpawnRequest{
		ACPAgent:               acp.AgentOpenCode,
		Slug:                   "restricted-worker",
		MCPServerPolicy:        acp.MCPServerPolicyMemorySearchWorker,
		SystemPromptExtensions: promptmodule.New("memory worker only"),
	})
	if err != nil {
		t.Fatal(err)
	}
	readOpenCodeInstructions(t, restricted.Cwd, "memory worker only\n")
	for i, session := range sessions {
		readOpenCodeInstructions(t, session.Cwd, "cwd: "+session.Cwd+"\n\nresumed "+session.SessionID+"\n")
		managers[i].Close()
	}
	files, err := filepath.Glob(filepath.Join(root, "acp", "opencode", "jaz-instructions*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("instruction files after shutdown = %v, %v", files, err)
	}
}

func readOpenCodeInstructions(t *testing.T, cwd, want string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cwd, "config-capture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal(data, &config); err != nil || len(config.Instructions) != 1 {
		t.Fatalf("launched configuration = %s, %v", data, err)
	}
	path := config.Instructions[0]
	instructions, err := os.ReadFile(path)
	if err != nil || string(instructions) != want {
		t.Fatalf("session instructions = %q, %v; want %q", instructions, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("instruction file permissions = %v, %v", info, err)
	}
	return path
}
