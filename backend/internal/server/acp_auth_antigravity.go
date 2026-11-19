package server

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"slices"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/terminal"
)

func (s *Server) antigravityLoginConfig() acp.AgentConfig {
	cfg, _ := s.acpAgentCatalog().Agent(acp.AgentAntigravity)
	cfg.AdapterBinDir = s.adapterBundleDir(cfg.ManagedAdapter)
	cfg.LoginBinDir = s.agentLoginBinDirs(cfg)
	return cfg
}

func (s *Server) handleAntigravityAuthMethods(w http.ResponseWriter, r *http.Request) {
	methods := acp.AntigravityLoginMethods(r.Context(), s.antigravityLoginConfig(), s.runtimeRoot())
	writeJSON(w, http.StatusOK, append([]string{}, methods...))
}

func (s *Server) handleAntigravityCloudLogin(w http.ResponseWriter, r *http.Request) {
	conn, err := httpapi.UpgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(64 << 10)
	ctx, cancel := context.WithTimeout(r.Context(), acpAuthLoginTimeout)
	defer cancel()
	fail := func(message string) {
		_ = writeTerminalMessage(conn, terminal.Message{Type: "error", Err: message})
	}
	if !s.antigravityLoginMu.TryLock() {
		fail(errAntigravityLoginRunning.Error())
		return
	}
	defer s.antigravityLoginMu.Unlock()
	cfg := s.antigravityLoginConfig()
	root := s.runtimeRoot()
	if !slices.Contains(acp.AntigravityLoginMethods(ctx, cfg, root), acp.AntigravityCloudProject) {
		fail("This Antigravity CLI does not offer Google Cloud sign-in.")
		return
	}
	if acp.ProbeAgentAuth(acp.AgentAntigravity, cfg, root, nil).Authenticated {
		fail("Disconnect the existing Antigravity account before signing in to Google Cloud.")
		return
	}
	inv := acp.AgentLoginInvocationFor(acp.AgentAntigravity, root, cfg.Auth, cfg.LoginBinDir, cfg.Env)
	cmd := exec.CommandContext(ctx, inv.Executable)
	cmd.Dir = inv.Cwd
	// The browser and CLI run on opposite ends of the terminal socket. Use the
	// CLI's SSH mode to give the browser link and code prompt to the client.
	inv.Env = map[string]string{"SSH_CONNECTION": "127.0.0.1 0 127.0.0.1 0"}
	cmd.Env = acpAuthLoginEnv(inv)
	term, err := terminal.OpenCommand(cmd, terminal.Size{Cols: queryUint16(r, "cols"), Rows: queryUint16(r, "rows")})
	if err != nil {
		fail(fmt.Sprintf("Could not start Antigravity sign-in: %v", err))
		return
	}
	defer term.Terminate()
	serveTerminal(conn, term, term.Terminate, func(msg terminal.Message) terminal.Message {
		if msg.Code == 0 {
			if err := s.verifyACPAuthLogin(acp.AgentAntigravity, cfg.Auth); err != nil {
				msg.Err = err.Error()
			}
		}
		return msg
	})
}
