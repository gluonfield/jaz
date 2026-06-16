//go:build acpprobe && !windows

package acp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

func TestLiveACPMultiSessionProbe(t *testing.T) {
	agent := firstNonEmpty(os.Getenv("ACP_PROBE_AGENT"), AgentCodex)
	timeout := 60 * time.Second
	if raw := os.Getenv("ACP_PROBE_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := firstNonEmpty(os.Getenv("ACP_PROBE_ROOT"), filepath.Join(os.Getenv("HOME"), ".jaz"))
	base := filepath.Join(root, "tmp", "acp-multisession-probe", strings.ReplaceAll(agent, "/", "-"))
	cwdA := filepath.Join(base, "a")
	cwdB := filepath.Join(base, "b")
	if err := os.MkdirAll(cwdA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cwdB, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := probeAgentConfig(t, agent)
	manager := NewManager(nil, Config{Root: root, Workspace: cwdA}, log.New(io.Discard))
	env := manager.processEnv(agent, cfg)
	if home := strings.TrimSpace(os.Getenv("ACP_PROBE_HOME")); home != "" {
		env["HOME"] = home
	}
	applyProbeEnvOverrides(env)

	totalStarted := time.Now()
	openStarted := time.Now()
	conn, cleanup := probeOpenConn(t, ctx, agent, cfg, env, cwdA)
	openDuration := time.Since(openStarted)
	defer cleanup()

	initStarted := time.Now()
	init := probeCall(t, ctx, conn, "1", acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
		ClientInfo: &acpschema.Implementation{
			Name:    "jaz-multisession-probe",
			Title:   "Jaz ACP Multisession Probe",
			Version: "0.1.0",
		},
		ClientCapabilities: &acpschema.ClientCapabilities{
			Meta: map[string]any{"terminal-auth": true},
			FS:   &acpschema.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
		},
	})
	initDuration := time.Since(initStarted)
	authDuration := time.Duration(0)
	if methodID, missing := autoAuthMethod(agent, init.Result, env); methodID != "" {
		authStarted := time.Now()
		_ = probeCall(t, ctx, conn, "2", acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID})
		authDuration = time.Since(authStarted)
	} else if len(missing) > 0 {
		t.Fatalf("missing auth for %s: %s", agent, strings.Join(missing, ", "))
	}

	firstStarted := time.Now()
	first := probeNewSession(t, ctx, conn, "3", cwdA)
	firstDuration := time.Since(firstStarted)
	modelDuration, effortDuration, modeDuration := probeApplyJazSessionConfig(t, ctx, conn, agent, first.SessionID, first.Modes, cfg)
	secondStarted := time.Now()
	second := probeNewSession(t, ctx, conn, "4", cwdB)
	secondDuration := time.Since(secondStarted)
	if first.SessionID == "" || second.SessionID == "" {
		t.Fatalf("empty session id: first=%q second=%q", first.SessionID, second.SessionID)
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("duplicate session id %q from one %s process", first.SessionID, agent)
	}
	t.Logf("%s accepted two session/new calls in one process: %s and %s", agent, first.SessionID, second.SessionID)
	t.Logf("%s timings: process_start=%s initialize=%s authenticate=%s first_session_new=%s set_model=%s set_effort=%s set_mode=%s second_session_new=%s total=%s",
		agent, openDuration, initDuration, authDuration, firstDuration, modelDuration, effortDuration, modeDuration, secondDuration, time.Since(totalStarted))
}

func probeNewSession(t *testing.T, ctx context.Context, conn jsonrpc.MessageConn, id string, cwd string) acpschema.NewSessionResponse {
	t.Helper()
	msg := probeCall(t, ctx, conn, id, acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{
		Cwd:        cwd,
		MCPServers: []acpschema.MCPServer{},
	})
	var resp acpschema.NewSessionResponse
	if err := json.Unmarshal(msg.Result, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func probeApplyJazSessionConfig(
	t *testing.T,
	ctx context.Context,
	conn jsonrpc.MessageConn,
	agent string,
	sessionID acpschema.SessionID,
	modes *acpschema.SessionModeState,
	cfg AgentConfig,
) (time.Duration, time.Duration, time.Duration) {
	t.Helper()
	policy := agentPolicyForAgent(agent)
	var modelDuration time.Duration
	model := configuredSessionModel(cfg.Model)
	if model != "" {
		started := time.Now()
		if policy.usesModelConfigOption() {
			_ = probeCall(t, ctx, conn, "5", acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
				SessionID: sessionID,
				ConfigID:  acpschema.SessionConfigID(policy.modelConfigID),
				Value:     acpschema.SessionConfigValueID(model),
			})
		} else {
			_ = probeCall(t, ctx, conn, "5", agentMethodSessionSetModel, setSessionModelRequest{
				SessionID: sessionID,
				ModelID:   model,
			})
		}
		modelDuration = time.Since(started)
	}

	var effortDuration time.Duration
	effort := policy.sessionConfigEffort(cfg.ReasoningEffort)
	if effort != "" && policy.usesReasoningEffortConfigOption() && !policy.effortEncodedInModel(cfg.Model) {
		started := time.Now()
		_ = probeCall(t, ctx, conn, "6", acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
			SessionID: sessionID,
			ConfigID:  acpschema.SessionConfigID(policy.reasoningEffortConfigID()),
			Value:     acpschema.SessionConfigValueID(effort),
		})
		effortDuration = time.Since(started)
	}

	var modeDuration time.Duration
	acpModes := modes
	if acpModes == nil && CanonicalAgentName(agent) == AgentGrok {
		acpModes = grokFallbackModes()
	}
	if acpModes != nil {
		target := executionModeForAgent(agent, acpModes.AvailableModes)
		if target != "" && string(acpModes.CurrentModeID) != target {
			started := time.Now()
			_ = probeCall(t, ctx, conn, "7", acpschema.AgentMethodSessionSetMode, acpschema.SetSessionModeRequest{
				SessionID: sessionID,
				ModeID:    acpschema.SessionModeID(target),
			})
			modeDuration = time.Since(started)
		}
	}
	return modelDuration, effortDuration, modeDuration
}
