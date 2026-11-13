package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/processenv"
)

func TestCopilotNativeLoginAndModelDiscovery(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(root, "copilot")
	if runtime.GOOS == "windows" {
		command += ".exe"
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"COPILOT_JAZ_TEST_CLI": root,
		"COPILOT_HOME":         filepath.Join(root, "native-profile"),
		"HTTP_PROXY":           "http://proxy.invalid:8080",
		"OPENAI_API_KEY":       "coordinator-key",
	} {
		t.Setenv(key, value)
	}
	cfg, _ := BuiltinAgents().Agent(AgentCopilot)
	cfg.Command = command
	cfg.LoginBinDir = root
	if status := ProbeAgentAuth(AgentCopilot, cfg, root, nil); status.Authenticated || !status.LoginCommandAvailable {
		t.Fatalf("signed-out native status = %#v", status)
	}
	invocation := AgentLoginInvocationFor(AgentCopilot, root, AgentAuthConfig{}, root, nil)
	login := exec.CommandContext(t.Context(), invocation.Executable, invocation.Args...)
	login.Env = processenv.List(invocation.Env)
	if err := login.Run(); err != nil {
		t.Fatalf("native login did not receive its profile/environment: %v", err)
	}
	status := ProbeAgentAuth(AgentCopilot, cfg, root, nil)
	if !status.Authenticated || status.AuthMode != AuthModeExistingCLI || status.RefreshOwner != RefreshOwnerAgentCLI {
		t.Fatalf("native login not reused = %#v", status)
	}
	manager := NewManager(nil, Config{Root: root, Agents: map[string]AgentConfig{AgentCopilot: cfg}}, nil)
	catalog, err := manager.DiscoverModels(t.Context(), AgentCopilot)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.Models[0].Value != "native-model" || catalog.Models[0].ContextLength != 240000 || !reflect.DeepEqual(catalog.Models[0].Reasoning.Efforts, []string{"low", "high"}) {
		t.Fatalf("native model metadata lost = %#v", catalog)
	}
	if catalog.Models[1].ContextLength != 0 || catalog.Models[1].Reasoning.Status != modelcatalog.ReasoningPending || catalog.DefaultModel != "" {
		t.Fatalf("missing native metadata was guessed = %#v", catalog)
	}
	if err := os.WriteFile(filepath.Join(root, "rpc-error"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DiscoverModels(t.Context(), AgentCopilot); err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("native error was missing or exposed provider data: %v", err)
	}
}

func TestCopilotInstructionsAppendAndCleanup(t *testing.T) {
	root := t.TempDir()
	env := map[string]string{"COPILOT_CUSTOM_INSTRUCTIONS_DIRS": "/native/instructions"}
	cleanup, err := configureCopilotPrompt(root, env, "Jaz extension")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	_, directory, ok := strings.Cut(env["COPILOT_CUSTOM_INSTRUCTIONS_DIRS"], ",")
	if !ok || !strings.HasPrefix(env["COPILOT_CUSTOM_INSTRUCTIONS_DIRS"], "/native/instructions,") {
		t.Fatalf("native instructions replaced: %#v", env)
	}
	data, err := os.ReadFile(filepath.Join(directory, "jaz.instructions.md"))
	if err != nil || string(data) != "Jaz extension" {
		t.Fatalf("Jaz extension unavailable: %q, %v", data, err)
	}
	cleanup()
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("instructions retained after process close: %v", err)
	}
}

func runFakeCopilotCLI(root string) int {
	if os.Getenv("COPILOT_HOME") != filepath.Join(root, "native-profile") || os.Getenv("HTTP_PROXY") != "http://proxy.invalid:8080" || os.Getenv("OPENAI_API_KEY") != "" {
		return 2
	}
	if reflect.DeepEqual(os.Args[1:], []string{"login", "--device-code"}) {
		if err := os.WriteFile(filepath.Join(root, "signed-in"), nil, 0o600); err != nil {
			return 3
		}
		return 0
	}
	if !reflect.DeepEqual(os.Args[1:], []string{"--headless", "--stdio", "--no-auto-update"}) {
		return 4
	}
	reader := bufio.NewReader(os.Stdin)
	headers, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		return 5
	}
	length, err := strconv.Atoi(headers.Get("Content-Length"))
	if err != nil {
		return 6
	}
	var request struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
	}
	if json.NewDecoder(io.LimitReader(reader, int64(length))).Decode(&request) != nil {
		return 7
	}
	var response []byte
	if _, err := os.Stat(filepath.Join(root, "rpc-error")); err == nil {
		response = []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"private-token"}}`)
	} else {
		var result any
		switch request.Method {
		case "auth.getStatus":
			_, err := os.Stat(filepath.Join(root, "signed-in"))
			result = map[string]bool{"isAuthenticated": err == nil}
		case "models.list":
			result = json.RawMessage(`{"models":[{"id":"native-model","name":"Native model","capabilities":{"limits":{"max_context_window_tokens":240000}},"supportedReasoningEfforts":["low","high"],"defaultReasoningEffort":"high"},{"id":"unknown-metadata","name":"Unknown metadata"}]}`)
		default:
			return 8
		}
		response, err = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		if err != nil {
			return 9
		}
	}
	_, err = fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(response), response)
	if err != nil {
		return 10
	}
	return 0
}
