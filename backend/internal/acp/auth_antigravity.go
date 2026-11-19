package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/processenv"
)

const (
	AntigravityGoogleAccount = "oauth-personal"
	AntigravityCloudProject  = "oauth-business"
)

func AntigravityLoginMethods(ctx context.Context, cfg AgentConfig, root string) []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	invocation := AgentLoginInvocationFor(AgentAntigravity, root, cfg.Auth, loginBinDirs(cfg), cfg.Env)
	if !invocation.Available || PrepareAgentLoginInvocation(AgentAntigravity, cfg.Auth, root, invocation) != nil {
		return nil
	}
	manager := NewManager(nil, Config{Root: root}, nil)
	defer manager.Close()
	cmd := exec.CommandContext(ctx, invocation.Executable, "acp", "--disable-telemetry")
	cmd.Env = processenv.List(manager.probeEnv(AgentAntigravity, cfg))
	cmd.Dir = invocation.Cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()
	peer := jsonrpc.NewPeer(stdio.New(stdout, stdin), nil)
	go func() {
		_ = peer.Serve(ctx)
	}()
	raw, err := peer.Call(ctx, acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersionNumber,
		ClientInfo:      &acpschema.Implementation{Name: "jaz", Title: "Jaz"},
	})
	var initialized struct {
		ProtocolVersion acpschema.ProtocolVersion `json:"protocolVersion"`
		AuthMethods     []agentAuthMethod         `json:"authMethods"`
	}
	if err != nil || json.Unmarshal(raw, &initialized) != nil || initialized.ProtocolVersion != acpschema.ProtocolVersionNumber {
		return nil
	}
	var methods []string
	for _, id := range []string{AntigravityGoogleAccount, AntigravityCloudProject} {
		if slices.ContainsFunc(initialized.AuthMethods, func(method agentAuthMethod) bool {
			return string(method.ID) == id && method.Type != "terminal"
		}) {
			methods = append(methods, id)
		}
	}
	return methods
}

func disconnectAntigravityAuth(ctx context.Context, cfg AgentConfig, root string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if cfg.ManagedAdapter != "" {
		command, err := antigravityExecutable(loginBinDirs(cfg))
		if err != nil {
			return err
		}
		cfg.useCommandLaunch(command, cfg.ManagedAdapterArgs)
	}
	manager := NewManager(nil, Config{Root: root}, nil)
	defer manager.Close()
	conn, err := manager.connect(ctx, AgentAntigravity, cfg, "", "", "", nil)
	if err != nil {
		return err
	}
	defer conn.close()
	var initialized acpschema.InitializeResponse
	if err := json.Unmarshal(conn.initRaw, &initialized); err != nil {
		return err
	}
	capabilities := initialized.AgentCapabilities
	if capabilities == nil || capabilities.Auth == nil || capabilities.Auth.Logout == nil {
		return fmt.Errorf("Antigravity does not advertise native sign out support")
	}
	if _, err := conn.peer.Call(ctx, acpschema.AgentMethodLogout, acpschema.LogoutRequest{}); err != nil {
		return fmt.Errorf("Antigravity sign out: %w", conn.withProcessStderr(err))
	}
	invalidateAntigravityAuthCache()
	return nil
}
