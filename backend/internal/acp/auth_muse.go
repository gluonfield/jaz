package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/processenv"
)

func museExecutable(env map[string]string) (string, error) {
	command := strings.TrimSpace(env["MUSE_CLI"])
	pathList := env["PATH"]
	if command == "" {
		command = "muse"
		dir := env["MUSE_INSTALL_DIR"]
		if dir == "" {
			dir = filepath.Join(env["HOME"], ".local", "bin")
			if runtime.GOOS == "windows" {
				dir = filepath.Join(env["LOCALAPPDATA"], "Programs", "muse")
			}
		}
		pathList += string(os.PathListSeparator) + dir
	}
	if strings.ContainsAny(command, `/\`) {
		resolved, err := filepath.Abs(command)
		if err != nil {
			return "", err
		}
		command = resolved
	}
	resolved, err := executableInPath(command, pathList)
	if err != nil && runtime.GOOS == "windows" && command == "muse" {
		resolved, err = executableInPath("muse.cmd", pathList)
	}
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func prepareMuseEnv(root string, env map[string]string) {
	processenv.PreserveHost(env, "META_API_KEY", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "LANG", "LC_ALL", "LC_CTYPE")
	for _, binding := range os.Environ() {
		key, value, _ := strings.Cut(binding, "=")
		if strings.HasPrefix(key, "MUSE_") && env[key] == "" {
			env[key] = value
		}
	}
	if command, err := museExecutable(env); err == nil {
		env["MUSE_CLI"] = command
	}
	if value, ok := explicitAgentAPIKey(AgentMuse, root, env); ok {
		env["META_API_KEY"] = value
	}
}

func disconnectMuseAuth(ctx context.Context, cfg AgentConfig, root string) error {
	env := NewManager(nil, Config{Root: root}, nil).probeEnv(AgentMuse, cfg)
	command, err := museExecutable(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command, args := launchCommand(command, []string{"logout"})
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = processenv.List(env)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Muse sign out: %w", err)
	}
	return nil
}

func museLoginInvocation(env map[string]string) AgentLoginInvocation {
	invocation := AgentLoginInvocation{
		Env:     env,
		Args:    []string{"login"},
		Display: "muse login",
	}
	command, err := museExecutable(env)
	invocation.Available = err == nil
	if err != nil {
		invocation.Reason = "Muse Code executable (muse) not found"
		return invocation
	}
	invocation.Executable, invocation.Args = launchCommand(command, invocation.Args)
	invocation.Display = shellCommand(command, "login")
	return invocation
}

func resolveMuseAuth(root string, env map[string]string) resolvedAgentAuth {
	status := resolvedAgentAuth{
		Config: AgentAuthConfig{Mode: AuthModeExistingCLI},
		Source: AuthModeExistingCLI,
	}
	if status.resolveAPIKey(AgentMuse, root, env) {
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
		return status
	}
	if value := strings.TrimSpace(env["META_API_KEY"]); value != "" {
		status.APIKeyValue = value
		status.APIKeySet = true
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
		return status
	}
	command, err := museExecutable(env)
	if err != nil {
		status.Reason = "Muse Code executable (muse) not found; install it from https://dev.meta.ai/docs/muse-code"
		return status
	}
	state, err := museAccountState(command, env)
	if err != nil {
		status.Reason = "Read Muse account status: " + err.Error()
		return status
	}
	switch state.State {
	case "accountLogin":
		status.markAuthenticated("muse_account", AuthKindOAuth)
	case "apiKey", "envKey":
		status.markAuthenticated("muse_account", AuthKindAPIKey)
	default:
		if state.CredentialRequired != nil && !*state.CredentialRequired {
			status.markAuthenticated("no_credential_required", AuthKindNone)
		} else {
			status.Reason = "Sign in with muse login or set JAZ_ACP_MUSE_API_KEY"
		}
	}
	return status
}

type museAccount struct {
	State              string `json:"state"`
	CredentialRequired *bool  `json:"credentialRequired"`
}

func museAccountState(command string, env map[string]string) (museAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command, args := launchCommand(command, []string{"serve"})
	cmd := exec.CommandContext(ctx, command, args...)
	prepareProcessCommand(cmd)
	process := newProcessSupervisor(cmd)
	cmd.Cancel = process.terminate
	cmd.Env = processenv.List(env)
	cmd.WaitDelay = processTerminateGrace + acpProcessStdioDrain
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return museAccount{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return museAccount{}, err
	}
	if err := cmd.Start(); err != nil {
		return museAccount{}, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
		_ = process.terminate()
	}()
	if err := process.started(); err != nil {
		_ = process.terminate()
		return museAccount{}, err
	}
	peer := jsonrpc.NewPeer(stdio.New(stdout, stdin), nil)
	defer peer.Close()
	go func() {
		_ = peer.Serve(ctx)
	}()
	if _, err := peer.Call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "jaz_auth", "version": "0.1.0"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}); err != nil {
		return museAccount{}, err
	}
	if err := peer.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return museAccount{}, err
	}
	raw, err := peer.Call(ctx, "account/read", map[string]any{})
	if err != nil {
		return museAccount{}, err
	}
	var state museAccount
	if err := json.Unmarshal(raw, &state); err != nil {
		return museAccount{}, fmt.Errorf("decode Muse account: %w", err)
	}
	return state, nil
}
