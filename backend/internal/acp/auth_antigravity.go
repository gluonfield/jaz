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

func AntigravityLoginMethods(ctx context.Context, cfg AgentConfig, root string) ([]string, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	invocation := AgentLoginInvocationFor(AgentAntigravity, root, cfg.Auth, loginBinDirs(cfg), cfg.Env)
	if !invocation.Available {
		return nil, nil
	}
	if err := PrepareAgentLoginInvocation(AgentAntigravity, cfg.Auth, root, invocation); err != nil {
		return nil, err
	}
	cfg.useCommandLaunch(invocation.Executable, []string{"acp", "--disable-telemetry"})
	var methods []string
	err := withAntigravityAccountPeer(ctx, cfg, root, invocation.Cwd, func(_ *jsonrpc.Peer, init acpschema.InitializeResponse) error {
		var advertised []agentAuthMethod
		for _, raw := range init.AuthMethods {
			var method agentAuthMethod
			if err := json.Unmarshal(raw, &method); err != nil {
				return err
			}
			advertised = append(advertised, method)
		}
		for _, id := range []string{AntigravityGoogleAccount, AntigravityCloudProject} {
			if slices.ContainsFunc(advertised, func(method agentAuthMethod) bool {
				return string(method.ID) == id && method.Type != "terminal"
			}) {
				methods = append(methods, id)
			}
		}
		return nil
	})
	return methods, err
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
	err := withAntigravityAccountPeer(ctx, cfg, root, "", func(peer *jsonrpc.Peer, init acpschema.InitializeResponse) error {
		caps := init.AgentCapabilities
		if caps == nil || caps.Auth == nil || caps.Auth.Logout == nil {
			return fmt.Errorf("Antigravity does not advertise native sign out support")
		}
		if _, err := peer.Call(ctx, acpschema.AgentMethodLogout, acpschema.LogoutRequest{}); err != nil {
			return fmt.Errorf("Antigravity sign out: %w", err)
		}
		return nil
	})
	if err == nil {
		invalidateAntigravityAuthCache()
	}
	return err
}

func withAntigravityAccountPeer(ctx context.Context, cfg AgentConfig, root, cwd string, run func(*jsonrpc.Peer, acpschema.InitializeResponse) error) (err error) {
	manager := NewManager(nil, Config{Root: root}, nil)
	defer manager.Close()
	env := manager.probeEnv(AgentAntigravity, cfg)
	var conn jsonrpc.MessageConn
	if cfg.URL != "" {
		remote, _, err := manager.openConn(ctx, AgentAntigravity, cfg, env, cwd, "")
		if err != nil {
			return err
		}
		conn = remote
		defer conn.Close()
	} else {
		processCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		command, args := launchCommand(cfg.Command, cfg.Args)
		addCommandDirToPath(env, command)
		cmd := exec.CommandContext(processCtx, command, args...)
		cmd.Env = processenv.List(env)
		cmd.Dir = cwd
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer stdin.Close()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		defer stdout.Close()
		stderr := newProcessStderrTail()
		cmd.Stderr = stderr
		cmd.Cancel = stdin.Close
		cmd.WaitDelay = 250 * time.Millisecond
		if err := cmd.Start(); err != nil {
			return err
		}
		defer func() {
			cancel()
			_ = cmd.Wait()
			if detail, _ := stderr.snapshot(); err != nil && detail != "" {
				err = fmt.Errorf("%w: %s", err, detail)
			}
		}()
		conn = stdio.New(stdout, stdin)
	}
	peer := jsonrpc.NewPeer(conn, nil)
	go func() {
		_ = peer.Serve(ctx)
	}()
	raw, err := peer.Call(ctx, acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersionNumber,
		ClientInfo:      &acpschema.Implementation{Name: "jaz", Title: "Jaz"},
	})
	if err != nil {
		return err
	}
	var initialized acpschema.InitializeResponse
	if err := json.Unmarshal(raw, &initialized); err != nil {
		return err
	}
	if initialized.ProtocolVersion != acpschema.ProtocolVersionNumber {
		return fmt.Errorf("unsupported ACP protocol version %d", initialized.ProtocolVersion)
	}
	return run(peer, initialized)
}
