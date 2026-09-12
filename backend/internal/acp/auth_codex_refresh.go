package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Codex owns token refresh and credential persistence. No thread or turn is created.
func (m *Manager) RefreshCodexOAuth(ctx context.Context, cfg AgentConfig, home string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	executable := ""
	if cfg.ManagedAdapter != "" && m.cfg.Adapters != nil {
		launch, err := m.cfg.Adapters.ResolveAdapter(ctx, cfg.ManagedAdapter)
		if err != nil {
			return fmt.Errorf("prepare Codex OAuth refresh: %w", err)
		}
		executable = launch.Env["CODEX_PATH"]
	}
	if executable == "" {
		var err error
		executable, err = resolveLoginExecutable(loginBinDirs(cfg), "codex")
		if err != nil {
			return fmt.Errorf("Codex CLI is required to refresh OpenAI OAuth")
		}
	}
	command := exec.CommandContext(ctx, executable, "app-server")
	command.Dir = home
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "OPENAI_API_KEY=") && !strings.HasPrefix(entry, "CODEX_HOME=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "CODEX_HOME="+home)
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		_ = input.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	encoder := json.NewEncoder(input)
	decoder := json.NewDecoder(output)
	call := func(id int, method string, params any) error {
		if err := encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return err
		}
		for {
			var reply struct {
				ID    int             `json:"id"`
				Error json.RawMessage `json:"error"`
			}
			if err := decoder.Decode(&reply); err != nil {
				return fmt.Errorf("Codex OAuth refresh did not complete")
			}
			if reply.ID != id {
				continue
			}
			if len(reply.Error) != 0 && string(reply.Error) != "null" {
				return fmt.Errorf("Codex could not refresh OpenAI OAuth; sign in again in Agents → Codex")
			}
			return nil
		}
	}
	if err := call(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "jaz", "version": "1.0"}}); err != nil {
		return err
	}
	if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return err
	}
	return call(2, "account/read", map[string]bool{"refreshToken": true})
}
