package acp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/processenv"
)

func TestMuseNativeAccountLoginAndDisconnect(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	relativeExecutable, err := filepath.Rel(root, executable)
	if err != nil {
		t.Fatal(err)
	}
	accountFile := filepath.Join(root, "account.json")
	cfg := AgentConfig{Command: executable, Env: map[string]string{
		"MUSE_CLI":          relativeExecutable,
		"MUSE_CHANNEL":      "test-channel",
		"MUSE_AUTH_PATH":    filepath.Join(root, "native-auth.json"),
		"MUSE_JAZ_TEST_CLI": root,
		"XDG_CONFIG_HOME":   root,
		"META_API_KEY":      "",
		"HTTP_PROXY":        "http://muse-proxy.invalid:8080",
		"LANG":              "C",
		"PATH":              filepath.Dir(executable),
	}}
	t.Setenv("META_API_KEY", "")
	t.Setenv("JAZ_ACP_MUSE_API_KEY", "")
	for _, state := range []struct {
		account       string
		authenticated bool
		kind          string
	}{
		{"{\"state\":\"loggedOut\",\"credentialRequired\":true}", false, ""},
		{"{\"state\":\"accountLogin\",\"credentialRequired\":true}", true, AuthKindOAuth},
		{"{\"state\":\"apiKey\",\"credentialRequired\":true}", true, AuthKindAPIKey},
		{"{\"state\":\"loggedOut\",\"credentialRequired\":false}", true, AuthKindNone},
	} {
		if err := os.WriteFile(accountFile, []byte(state.account), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(filepath.Join(root, "probe-closed"))
		status := ProbeAgentAuth(AgentMuse, cfg, root, nil)
		if status.Authenticated != state.authenticated || status.AuthKind != state.kind || !status.LoginCommandAvailable {
			t.Fatalf("native account %s => %#v", state.account, status)
		}
		if _, err := os.Stat(filepath.Join(root, "probe-closed")); err != nil {
			t.Fatalf("account probe terminated before native shutdown: %v", err)
		}
	}
	invocation := AgentLoginInvocationFor(AgentMuse, root, AgentAuthConfig{}, "", cfg.Env)
	if invocation.Executable != executable || invocation.Env["XDG_CONFIG_HOME"] != root {
		t.Fatalf("login selected a different native profile: %#v", invocation)
	}
	if err := PrepareAgentLoginInvocation(AgentMuse, AgentAuthConfig{}, root, invocation); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"test-channel", "native-auth.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("native environment value created a profile directory: %s, %v", name, err)
		}
	}
	cmd := exec.CommandContext(t.Context(), invocation.Executable, invocation.Args...)
	cmd.Env = processenv.List(invocation.Env)
	if err := cmd.Run(); err != nil {
		t.Fatalf("native login rejected its environment: %v", err)
	}
	if status := ProbeAgentAuth(AgentMuse, cfg, root, nil); !status.Authenticated || status.AuthKind != AuthKindOAuth {
		t.Fatalf("native login was not available to the runtime: %#v", status)
	}
	if err := DisconnectAgentAuth(t.Context(), AgentMuse, cfg, root, ""); err != nil {
		t.Fatal(err)
	}
	if status := ProbeAgentAuth(AgentMuse, cfg, root, nil); status.Authenticated {
		t.Fatalf("Muse remained signed in: %#v", status)
	}
	cfg.Env["MUSE_CLI"] = filepath.Base(executable)
	if status := ProbeAgentAuth(AgentMuse, cfg, root, nil); !status.LoginCommandAvailable || status.Authenticated {
		t.Fatalf("native executable was not resolved through the configured PATH: %#v", status)
	}
	managedBin := t.TempDir()
	name := "muse"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	managedCLI := filepath.Join(managedBin, name)
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedCLI, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Env["MUSE_CLI"] = ""
	cfg.Env["PATH"] = filepath.Dir(executable)
	cfg.LoginBinDir = managedBin
	status := ProbeAgentAuth(AgentMuse, cfg, root, nil)
	if !status.LoginCommandAvailable {
		t.Fatalf("managed native CLI was unavailable: %#v", status)
	}
	invocation = AgentLoginInvocationFor(AgentMuse, root, AgentAuthConfig{}, managedBin, cfg.Env)
	if !invocation.Available || invocation.Executable != managedCLI || invocation.Env["MUSE_CLI"] != managedCLI {
		t.Fatalf("login did not select the managed native CLI: %#v", invocation)
	}
}

func runFakeMuseCLI(root string) int {
	if len(os.Args) != 2 || os.Getenv("XDG_CONFIG_HOME") != root {
		return 2
	}
	accountFile := filepath.Join(root, "account.json")
	if os.Args[1] == "login" {
		if os.Getenv("HTTP_PROXY") != "http://muse-proxy.invalid:8080" || os.Getenv("LANG") != "C" || os.Getenv("PATH") != filepath.Dir(os.Args[0]) {
			return 9
		}
		if err := os.WriteFile(accountFile, []byte("{\"state\":\"accountLogin\",\"credentialRequired\":true}"), 0o600); err != nil {
			return 10
		}
		return 0
	}
	if os.Args[1] == "logout" {
		if err := os.WriteFile(accountFile, []byte("{\"state\":\"loggedOut\",\"credentialRequired\":true}"), 0o600); err != nil {
			return 3
		}
		return 0
	}
	if os.Args[1] != "serve" {
		return 4
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for _, method := range []string{"initialize", "initialized", "account/read"} {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := decoder.Decode(&request); err != nil || request.Method != method {
			return 5
		}
		if method == "initialized" {
			continue
		}
		result := json.RawMessage("{}")
		if method == "account/read" {
			data, err := os.ReadFile(accountFile)
			if err != nil {
				return 6
			}
			result = data
		}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			return 7
		}
	}
	var extra json.RawMessage
	if decoder.Decode(&extra) == nil {
		return 8
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(root, "probe-closed"), []byte("closed"), 0o600); err != nil {
		return 11
	}
	return 0
}
