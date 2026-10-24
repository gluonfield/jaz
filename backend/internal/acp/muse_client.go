package acp

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/processenv"
	"github.com/wins/jaz/backend/internal/subprocess"
)

func museCall(ctx context.Context, command string, env map[string]string, method string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command, args := launchCommand(command, []string{"serve"})
	cmd := exec.CommandContext(ctx, command, args...)
	process := subprocess.New(cmd)
	cmd.Env = processenv.List(env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
		_ = process.Terminate()
	}()
	if err := process.Started(); err != nil {
		_ = process.Terminate()
		return nil, err
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
		return nil, err
	}
	if err := peer.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return nil, err
	}
	return peer.Call(ctx, method, map[string]any{})
}
