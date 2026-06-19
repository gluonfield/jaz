//go:build acpprobe && !windows

package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/promptmodule"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestLiveGrokRulesPromptExtensionProbe(t *testing.T) {
	timeout := 45 * time.Second
	if raw := os.Getenv("ACP_PROBE_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := firstNonEmpty(os.Getenv("ACP_PROBE_ROOT"), filepath.Join(os.Getenv("HOME"), ".jaz"))
	cwd := filepath.Join(root, "tmp", "grok-rules-probe", fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := probeAgentConfig(t, AgentGrok)
	manager := NewManager(nil, Config{Root: root, Workspace: cwd}, nil)
	env := manager.processEnv(AgentGrok, cfg)
	if home := strings.TrimSpace(os.Getenv("ACP_PROBE_HOME")); home != "" {
		env["HOME"] = home
	}
	applyProbeEnvOverrides(env)

	conn, cleanup := probeOpenConn(t, ctx, AgentGrok, cfg, env, cwd)
	defer cleanup()

	init := probeCall(t, ctx, conn, "1", acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
		ClientInfo: &acpschema.Implementation{
			Name:    "jaz-grok-rules-probe",
			Title:   "Jaz Grok Rules Probe",
			Version: "0.1.0",
		},
		ClientCapabilities: &acpschema.ClientCapabilities{
			Meta: map[string]any{"terminal-auth": true},
			FS:   &acpschema.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
		},
	})
	if methodID, missing := autoAuthMethod(AgentGrok, init.Result, env); methodID != "" {
		_ = probeCall(t, ctx, conn, "2", acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID})
	} else if len(missing) > 0 {
		t.Fatalf("missing auth for grok: %s", strings.Join(missing, ", "))
	}

	marker := fmt.Sprintf("JAZ_GROK_RULES_PROBE_%d", time.Now().UnixNano())
	session := probeCall(t, ctx, conn, "3", acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{
		Meta: map[string]any{
			"rules": "Rules probe marker: " + marker,
		},
		Cwd:        cwd,
		MCPServers: []acpschema.MCPServer{},
	})
	var sessionResp acpschema.NewSessionResponse
	if err := json.Unmarshal(session.Result, &sessionResp); err != nil {
		t.Fatal(err)
	}
	if sessionResp.SessionID == "" {
		t.Fatal("empty session id")
	}

	sessionDir, err := waitForGrokSessionDir(ctx, string(sessionResp.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(sessionDir, "system_prompt.txt")
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read grok system prompt %s: %v", promptPath, err)
	}
	if !strings.Contains(string(prompt), marker) {
		t.Fatalf("grok system prompt did not contain rules marker %q in %s", marker, promptPath)
	}
	t.Logf("grok accepted _meta.rules; marker %s found in %s", marker, promptPath)
}

func TestLiveGrokManagerPromptExtensionProbe(t *testing.T) {
	timeout := 45 * time.Second
	if raw := os.Getenv("ACP_PROBE_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := firstNonEmpty(os.Getenv("ACP_PROBE_ROOT"), filepath.Join(os.Getenv("HOME"), ".jaz"))
	base := filepath.Join(root, "tmp", "grok-manager-prompt-probe", fmt.Sprintf("%d", time.Now().UnixNano()))
	cwd := filepath.Join(base, "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := jsonstore.New(filepath.Join(base, "store"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := probeAgentConfig(t, AgentGrok)
	env := map[string]string{}
	applyProbeEnvOverrides(env)
	marker := fmt.Sprintf("JAZ_GROK_MANAGER_PROMPT_PROBE_%d", time.Now().UnixNano())
	manager := NewManager(store, Config{
		Root:         root,
		Workspace:    cwd,
		Agents:       map[string]AgentConfig{AgentGrok: cfg},
		Env:          env,
		SystemPrompt: testPrompt("manager base marker: " + marker),
	}, nil)
	defer manager.Close()

	spawned, err := manager.Spawn(ctx, SpawnRequest{
		ACPAgent:               AgentGrok,
		Slug:                   "grok-manager-prompt-probe",
		ArtifactSurface:        "widget",
		SystemPromptExtensions: promptmodule.New("manager extension marker: " + marker),
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionDir, err := waitForGrokSessionDir(ctx, spawned.Session.RuntimeRef.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(sessionDir, "system_prompt.txt")
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read grok system prompt %s: %v", promptPath, err)
	}
	for _, want := range []string{"manager base marker: " + marker, "manager extension marker: " + marker} {
		if !strings.Contains(string(prompt), want) {
			t.Fatalf("grok manager system prompt missing %q in %s", want, promptPath)
		}
	}
	t.Logf("jaz manager delivered Grok prompt extension marker %s in %s", marker, promptPath)
}

func waitForGrokSessionDir(ctx context.Context, sessionID string) (string, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if path, ok := findGrokSessionDir(sessionID); ok {
			return path, nil
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("grok session dir %s not found: %w", sessionID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func findGrokSessionDir(sessionID string) (string, bool) {
	roots := grokSessionRoots()
	for _, root := range roots {
		var found string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			if d.IsDir() && filepath.Base(path) == sessionID {
				found = path
				return fs.SkipDir
			}
			return nil
		})
		if found != "" {
			return found, true
		}
	}
	return "", false
}

func grokSessionRoots() []string {
	seen := map[string]bool{}
	var roots []string
	add := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			return
		}
		seen[root] = true
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	if home := strings.TrimSpace(os.Getenv("ACP_PROBE_HOME")); home != "" {
		add(filepath.Join(home, ".grok", "sessions"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".grok", "sessions"))
	}
	return roots
}
