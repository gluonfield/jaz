package acp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestKimiSystemPromptStaysInSessionMetadataAcrossManagerRestart(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const marker = "Kimi system marker"
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	root := t.TempDir()
	workspace := t.TempDir()
	newManager := func() *acp.Manager {
		return acp.NewManager(store, acp.Config{
			Root:         root,
			Workspace:    workspace,
			SystemPrompt: staticPrompt(marker),
			Agents: map[string]acp.AgentConfig{
				acp.AgentKimi: {
					Command: os.Args[0],
					Args:    []string{"-test.run=TestFakeACPAgentProcess"},
					Env: map[string]string{
						"JAZ_FAKE_ACP_AGENT":       "1",
						"JAZ_FAKE_ACP_LOAD":        "1",
						"JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
					},
				},
			},
		}, log.New(io.Discard))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	manager := newManager()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentKimi, Slug: "kimi-system"})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWaitKimi(t, ctx, manager, spawned.SessionID, "first")
	manager.Close()

	manager = newManager()
	t.Cleanup(manager.Close)
	sendAndWaitKimi(t, ctx, manager, spawned.SessionID, "after manager restart")

	requests := readFakeACPRequests(t, requestLog)
	attachments := 0
	prompts := 0
	for _, request := range requests {
		switch request.Method {
		case "session/new", "session/load":
			attachments++
			var params struct {
				Meta map[string]any `json:"_meta"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatal(err)
			}
			prompt, _ := params.Meta["systemPrompt"].(string)
			if !strings.Contains(prompt, marker) {
				t.Fatalf("%s omitted Kimi system prompt: %s", request.Method, request.Params)
			}
		case "session/prompt":
			prompts++
			if strings.Contains(string(request.Params), marker) {
				t.Fatalf("Kimi system prompt leaked into user history: %s", request.Params)
			}
		}
	}
	if attachments < 2 || prompts != 2 {
		t.Fatalf("Kimi request counts = %d session attaches, %d prompts; want at least 2 and exactly 2", attachments, prompts)
	}
}

func sendAndWaitKimi(t *testing.T, ctx context.Context, manager *acp.Manager, sessionID, message string) {
	t.Helper()
	if _, err := manager.Send(ctx, acp.SendRequest{Session: sessionID, Message: message, Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: sessionID, Timeout: 10 * time.Second})
	if err != nil || job.State != acp.StateIdle {
		t.Fatalf("Kimi %q turn = %#v, %v", message, job, err)
	}
}

type fakeACPRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func readFakeACPRequests(t *testing.T, path string) []fakeACPRequest {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var requests []fakeACPRequest
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var request fakeACPRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return requests
}
