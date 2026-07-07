//go:build acpprobe && !windows

package acp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
)

func TestLiveACPProbe(t *testing.T) {
	agent := firstNonEmpty(os.Getenv("ACP_PROBE_AGENT"), AgentCodex)
	prompt := firstNonEmpty(os.Getenv("ACP_PROBE_PROMPT"), "Plan building a single static HTML page about koalas. Ask me clarifying questions if useful. Do not edit files.")
	timeout := 90 * time.Second
	if raw := os.Getenv("ACP_PROBE_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := firstNonEmpty(os.Getenv("ACP_PROBE_ROOT"), filepath.Join(os.Getenv("HOME"), ".jaz"))
	cwd := firstNonEmpty(os.Getenv("ACP_PROBE_CWD"), filepath.Join(root, "workspaces", "default"))
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := probeAgentConfig(t, agent)
	manager := NewManager(nil, Config{Root: root, Workspace: cwd}, log.New(io.Discard))
	env, err := manager.processEnvPreparedForSurface(ctx, agent, cfg, cwd, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if home := strings.TrimSpace(os.Getenv("ACP_PROBE_HOME")); home != "" {
		env["HOME"] = home
	}
	applyProbeEnvOverrides(env)
	conn, cleanup := probeOpenConn(t, ctx, agent, cfg, env, cwd)
	defer cleanup()

	init := probeCall(t, ctx, conn, "1", acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
		ClientInfo: &acpschema.Implementation{
			Name:    "jaz-probe",
			Title:   "Jaz ACP Probe",
			Version: "0.1.0",
		},
		ClientCapabilities: &acpschema.ClientCapabilities{
			Meta: map[string]any{
				"terminal-auth": true,
			},
			FS:       &acpschema.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
	})
	t.Logf("initialize result: %s", init.Result)
	if methodID, missing := autoAuthMethod(agent, init.Result, env); methodID != "" {
		auth := probeCall(t, ctx, conn, "2", acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID})
		t.Logf("authenticate result: %s", auth.Result)
	} else if len(missing) > 0 {
		t.Fatalf("missing auth for %s: %s", agent, strings.Join(missing, ", "))
	}

	session := probeCall(t, ctx, conn, "3", acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{
		Cwd:        cwd,
		MCPServers: []acpschema.MCPServer{},
	})
	t.Logf("session/new result: %s", session.Result)
	var sessionResp acpschema.NewSessionResponse
	if err := json.Unmarshal(session.Result, &sessionResp); err != nil {
		t.Fatal(err)
	}
	if sessionResp.SessionID == "" {
		t.Fatal("empty session id")
	}
	probeApplyConfiguredSessionOptions(t, ctx, conn, agent, sessionResp.SessionID)
	availableModes := []acpschema.SessionMode(nil)
	if sessionResp.Modes != nil {
		availableModes = sessionResp.Modes.AvailableModes
	}
	if modeID := preferredBaselineModeID(agent, availableModes); modeID != "" && (sessionResp.Modes == nil || string(sessionResp.Modes.CurrentModeID) != modeID) {
		setMode := probeCall(t, ctx, conn, "4", acpschema.AgentMethodSessionSetMode, acpschema.SetSessionModeRequest{
			SessionID: sessionResp.SessionID,
			ModeID:    acpschema.SessionModeID(modeID),
		})
		t.Logf("session/set_mode(%s) result: %s", modeID, setMode.Result)
	}
	if os.Getenv("ACP_PROBE_SKIP_PROMPT") == "1" {
		t.Log("skipping prompt")
		return
	}
	if os.Getenv("ACP_PROBE_SKIP_PLAN_MODE") == "1" {
		t.Log("skipping plan mode switch")
	} else if modeID := planModeID(availableModes); modeID != "" {
		setMode := probeCall(t, ctx, conn, "5", acpschema.AgentMethodSessionSetMode, acpschema.SetSessionModeRequest{
			SessionID: sessionResp.SessionID,
			ModeID:    acpschema.SessionModeID(modeID),
		})
		t.Logf("session/set_mode(%s) result: %s", modeID, setMode.Result)
	} else {
		t.Log("agent did not expose plan mode")
	}

	final := probeCall(t, ctx, conn, "6", acpschema.AgentMethodSessionPrompt, map[string]any{
		"sessionId": sessionResp.SessionID,
		"prompt": []any{
			map[string]any{"type": "text", "text": prompt},
		},
	})
	t.Logf("session/prompt result: %s", final.Result)
}

func probeApplyConfiguredSessionOptions(t *testing.T, ctx context.Context, conn jsonrpc.MessageConn, agent string, sessionID acpschema.SessionID) {
	t.Helper()
	policy := agentPolicyForAgent(agent)
	rawModel := strings.TrimSpace(os.Getenv("ACP_PROBE_MODEL"))
	if CanonicalAgentName(agent) == AgentOpenCode {
		rawModel = ""
	}
	effort := strings.TrimSpace(os.Getenv("ACP_PROBE_REASONING_EFFORT"))
	options := sessionConfigOptionsState{}
	if rawModel != "" {
		model := configuredSessionModel(rawModel)
		if policy.usesModelConfigOption() {
			setModel := probeCall(t, ctx, conn, "10", acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
				SessionID: sessionID,
				ConfigID:  acpschema.SessionConfigID(policy.modelConfigID),
				Value:     acpschema.SessionConfigValueID(model),
			})
			t.Logf("session/set_config_option(model=%s) result: %s", model, setModel.Result)
			options = parseSessionConfigOptions(setModel.Result)
		} else {
			setModel := probeCall(t, ctx, conn, "10", agentMethodSessionSetModel, setSessionModelRequest{
				SessionID: sessionID,
				ModelID:   model,
			})
			t.Logf("session/set_model(%s) result: %s", model, setModel.Result)
			options = parseSessionConfigOptions(setModel.Result)
		}
	}
	if effort != "" && (policy.usesReasoningEffortConfigOption() || options.effortConfigID != "") && !policy.effortEncodedInModel(rawModel) {
		configID := policy.reasoningEffortConfigID()
		if options.effortConfigID != "" {
			configID = options.effortConfigID
		}
		setEffort := probeCall(t, ctx, conn, "11", acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
			SessionID: sessionID,
			ConfigID:  acpschema.SessionConfigID(configID),
			Value:     acpschema.SessionConfigValueID(effort),
		})
		t.Logf("session/set_config_option(%s=%s) result: %s", configID, effort, setEffort.Result)
	}
}

func applyProbeEnvOverrides(env map[string]string) {
	if value := strings.TrimSpace(os.Getenv("ACP_PROBE_CLAUDE_EXECUTABLE")); value != "" {
		env["CLAUDE_CODE_EXECUTABLE"] = value
	}
	if value := strings.TrimSpace(os.Getenv("ACP_PROBE_CLAUDE_CONFIG_DIR")); value != "" {
		env["CLAUDE_CONFIG_DIR"] = value
	}
	for _, key := range []string{
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_MODEL",
		"CLAUDE_CODE_EXECUTABLE",
		"CLAUDE_CONFIG_DIR",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_REMOTE",
		"OPENROUTER_API_KEY",
		"XAI_API_KEY",
	} {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

func probeAgentConfig(t *testing.T, agent string) AgentConfig {
	t.Helper()
	agent = CanonicalAgentName(agent)
	cfg, ok := BuiltinAgents()[agent]
	if !ok {
		t.Fatalf("unknown probe agent %q", agent)
	}
	switch agent {
	case AgentCodex:
		command := strings.TrimSpace(os.Getenv("ACP_PROBE_CODEX_COMMAND"))
		if command != "" {
			cfg.Command = command
			cfg.Args = strings.Fields(strings.TrimSpace(os.Getenv("ACP_PROBE_CODEX_ARGS")))
		}
	case AgentClaude:
		pkg := strings.TrimSpace(os.Getenv("ACP_PROBE_CLAUDE_PACKAGE"))
		if pkg != "" {
			cfg.Command = "npx"
			cfg.Args = []string{"-y", pkg}
		}
	case AgentGrok:
		command := strings.TrimSpace(os.Getenv("ACP_PROBE_GROK_COMMAND"))
		if command != "" {
			cfg.Command = command
			cfg.Args = strings.Fields(strings.TrimSpace(os.Getenv("ACP_PROBE_GROK_ARGS")))
		}
	}
	if model := strings.TrimSpace(os.Getenv("ACP_PROBE_MODEL")); model != "" {
		cfg.Model = model
	}
	if effort := strings.TrimSpace(os.Getenv("ACP_PROBE_REASONING_EFFORT")); effort != "" {
		cfg.ReasoningEffort = effort
	}
	return cfg
}

func probeOpenConn(t *testing.T, ctx context.Context, agent string, cfg AgentConfig, env map[string]string, cwd string) (jsonrpc.MessageConn, func()) {
	t.Helper()
	command, args, err := processCommand(agent, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	command, args = launchCommand(command, args)
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = envList(env)
	cmd.Dir = cwd
	prepareProcessCommand(cmd)
	process := newProcessSupervisor(cmd)
	cmd.Cancel = process.terminate
	cmd.WaitDelay = acpProcessStdioDrain
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := process.started(); err != nil {
		_ = process.terminate()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	conn := stdio.New(stdout, stdin)
	return conn, func() {
		_ = stdin.Close()
		_ = process.terminate()
		_ = cmd.Wait()
	}
}

func probeCall(t *testing.T, ctx context.Context, conn jsonrpc.MessageConn, id string, method string, params any) *jsonrpc.Message {
	t.Helper()
	req, err := jsonrpc.NewRequest(json.RawMessage(id), method, params)
	if err != nil {
		t.Fatal(err)
	}
	probeLogMessage(t, "client -> agent", req)
	if err := conn.Send(ctx, req); err != nil {
		t.Fatal(err)
	}
	for {
		msg, err := conn.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		probeLogMessage(t, "agent -> client", msg)
		if msg.IsRequest() {
			probeHandleClientRequest(t, ctx, conn, msg)
			continue
		}
		if msg.IsResponse() && msg.ID != nil && string(*msg.ID) == id {
			if msg.Error != nil {
				t.Fatalf("%s failed: %v", method, msg.Error)
			}
			return msg
		}
	}
}

func probeHandleClientRequest(t *testing.T, ctx context.Context, conn jsonrpc.MessageConn, msg *jsonrpc.Message) {
	t.Helper()
	var result any
	var rpcErr *jsonrpc.Error
	switch msg.Method {
	case acpschema.ClientMethodSessionRequestPermission:
		result = acpschema.RequestPermissionResponseCancelled()
	case acpschema.ClientMethodFSReadTextFile:
		result = acpschema.ReadTextFileResponse{}
	case acpschema.ClientMethodFSWriteTextFile:
		result = acpschema.WriteTextFileResponse{}
	case acpschema.ClientMethodTerminalKill, acpschema.ClientMethodTerminalRelease:
		result = map[string]any{}
	case acpschema.ClientMethodTerminalCreate, acpschema.ClientMethodTerminalOutput, acpschema.ClientMethodTerminalWaitForExit:
		rpcErr = jsonrpc.InternalError("terminal support is disabled in live probe", nil)
	default:
		rpcErr = jsonrpc.MethodNotFound(msg.Method)
	}
	var resp *jsonrpc.Message
	var err error
	if rpcErr != nil {
		resp, err = jsonrpc.NewErrorResponse(*msg.ID, rpcErr)
	} else {
		resp, err = jsonrpc.NewResult(*msg.ID, result)
	}
	if err != nil {
		t.Fatal(err)
	}
	probeLogMessage(t, "client -> agent", resp)
	if err := conn.Send(ctx, resp); err != nil {
		t.Fatal(err)
	}
}

func probeLogMessage(t *testing.T, prefix string, msg *jsonrpc.Message) {
	t.Helper()
	if os.Getenv("ACP_PROBE_LOG_JSON") != "1" {
		return
	}
	raw, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s:\n%s", prefix, raw)
}
