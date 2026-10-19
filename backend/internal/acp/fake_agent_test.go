package acp_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

var fakeSubagentReport = regexp.MustCompile(`subagent (\w+)=(\w+)`)

// fakeCommandOffer makes a prompt advertise commands with or without compact.
var fakeCommandOffer = regexp.MustCompile(`(offer|withdraw) compact`)

// fakeContextFill makes a prompt report its context as used/size tokens.
var fakeContextFill = regexp.MustCompile(`context (\d+)/(\d+)`)

// fakeSessionID is the provider session id the fake agent reports; agents
// that run side by side need their own, as real ones have.
var fakeSessionID = "fake-session"

func TestFakeACPAgentProcess(t *testing.T) {
	if os.Getenv("JAZ_FAKE_ACP_AGENT") != "1" {
		return
	}
	if os.Getenv("JAZ_FAKE_ACP_UNIQUE_SESSION") == "1" {
		fakeSessionID = fmt.Sprintf("fake-session-%d", os.Getpid())
	}
	if path := os.Getenv("JAZ_FAKE_ACP_MUSE_CONFIG_CAPTURE"); path != "" {
		if err := os.WriteFile(path, []byte(os.Getenv("XDG_CONFIG_HOME")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("JAZ_FAKE_ACP_MUSE_SHUTDOWN_CAPTURE") != "" {
		signal.Ignore(syscall.SIGTERM)
	}
	_, authErr := os.Stat(os.Getenv("JAZ_FAKE_ACP_AUTH_REQUIRED_FILE"))
	authRequired := authErr == nil
	if path := os.Getenv("JAZ_FAKE_ACP_START_LOG"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fmt.Fprintln(file, os.Getpid())
		_ = file.Close()
	}
	// Startup waits while a hold file sits in the agent's directory, as a
	// slow login refresh holds an agent up.
	fakeHold(os.Getenv("JAZ_FAKE_ACP_START_HOLD"))
	launch, codex := readFakeCodexLaunch(t)
	if os.Getenv("JAZ_FAKE_ACP_EXPECT_CODEX_GOALS_DISABLED") == "1" {
		disabled, ok := launch.Features["goals"]
		if !ok || disabled {
			t.Fatalf("Codex config = %#v, want features.goals=false", launch)
		}
	}
	if want := os.Getenv("JAZ_FAKE_ACP_EXPECT_CODEX_METADATA"); want != "" {
		var metadata struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(os.Getenv("JAZ_CODEX_MODEL_METADATA")), &metadata); err != nil || metadata.ID != want {
			t.Fatalf("Codex model metadata = %#v, %v; want %q", metadata, err, want)
		}
	}
	expectsNoMetadata := os.Getenv("JAZ_FAKE_ACP_EXPECT_NO_CODEX_METADATA") == "1"
	if metadata := os.Getenv("JAZ_CODEX_MODEL_METADATA"); expectsNoMetadata && metadata != "" {
		t.Fatalf("unexpected Codex model metadata %q", metadata)
	}
	if msg := os.Getenv("JAZ_FAKE_ACP_EXIT_BEFORE_INIT"); msg != "" {
		_, _ = fmt.Fprintln(os.Stderr, msg)
		os.Exit(2)
	}
	// The first process dies on its way up, as one killed during startup.
	if path := os.Getenv("JAZ_FAKE_ACP_EXIT_ONCE"); path != "" {
		if _, err := os.Stat(path); err != nil {
			_ = os.WriteFile(path, nil, 0o600)
			_, _ = fmt.Fprintln(os.Stderr, "killed during startup")
			os.Exit(2)
		}
	}
	conn := stdio.New(os.Stdin, os.Stdout)
	currentMode := os.Getenv("JAZ_FAKE_ACP_CURRENT_MODE")
	if currentMode == "" {
		currentMode = "auto"
	}
	// Codex starts a thread on the model its launch names, else on its default.
	currentModel, currentEffort := launch.Model, launch.Effort
	if codex && currentModel == "" {
		currentModel = fakeDefaultModel()
	}
	startedModel, lastTurnModel := "", ""
	currentCollaborationMode := "default"
	messageSeq := 0
	nextMessageID := func(kind string) string {
		messageSeq++
		return fmt.Sprintf("fake:%s:%d", kind, messageSeq)
	}
	var pendingPrompt *jsonrpc.Message
	grokInterjections := 0
	cancelArrived := false
	for {
		msg, err := conn.Receive(context.Background())
		if err != nil {
			if path := os.Getenv("JAZ_FAKE_ACP_MUSE_SHUTDOWN_CAPTURE"); path != "" {
				time.Sleep(25 * time.Millisecond)
				rules, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "muse", "AGENTS.md"))
				if err != nil {
					rules = []byte(err.Error())
				}
				if err := os.WriteFile(path, rules, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			os.Exit(0)
		}
		logFakeACPRequest(t, msg)
		if !msg.IsRequest() {
			if msg.Method == "session/cancel" {
				if pendingPrompt != nil {
					sendResult(conn, pendingPrompt, map[string]any{"stopReason": fakeCancelStopReason()})
					pendingPrompt = nil
				} else {
					cancelArrived = true
				}
			}
			continue
		}
		switch msg.Method {
		case "initialize":
			if os.Getenv("JAZ_FAKE_ACP_EXPECT_TERMINAL_AUTH") == "1" {
				var req struct {
					ClientCapabilities struct {
						Meta        map[string]any `json:"_meta"`
						Elicitation struct {
							Form *struct{} `json:"form"`
						} `json:"elicitation"`
					} `json:"clientCapabilities"`
				}
				if err := json.Unmarshal(msg.Params, &req); err != nil || req.ClientCapabilities.Meta["terminal-auth"] != true || req.ClientCapabilities.Meta["terminal_output_delta"] != true {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing terminal capabilities", nil))
					_ = conn.Send(context.Background(), resp)
					continue
				}
				if os.Getenv("JAZ_FAKE_ACP_EXPECT_ELICITATION_FORM") == "1" && req.ClientCapabilities.Elicitation.Form == nil {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing form elicitation capability", nil))
					_ = conn.Send(context.Background(), resp)
					continue
				}
			}
			capabilities := map[string]any{
				"loadSession": os.Getenv("JAZ_FAKE_ACP_LOAD") != "0",
			}
			if os.Getenv("JAZ_FAKE_ACP_RESUME") == "1" {
				capabilities["sessionCapabilities"] = map[string]any{
					"resume": map[string]any{},
				}
			}
			if os.Getenv("JAZ_FAKE_ACP_MCP_HTTP") == "1" {
				capabilities["mcpCapabilities"] = map[string]any{"http": true}
			}
			if os.Getenv("JAZ_FAKE_ACP_PROMPT_QUEUEING") == "1" {
				capabilities["_meta"] = map[string]any{
					"claudeCode": map[string]any{"promptQueueing": true},
				}
			}
			response := map[string]any{
				"protocolVersion":   1,
				"agentInfo":         map[string]any{"name": "fake-agent", "version": "test"},
				"agentCapabilities": capabilities,
			}
			if os.Getenv("JAZ_FAKE_ACP_NATIVE_STEERING") == "1" {
				response["_meta"] = map[string]any{
					"steering": map[string]any{"supported": true, "waitForCompletion": true},
				}
			}
			if os.Getenv("JAZ_FAKE_ACP_GROK_INTERJECT") == "1" {
				response["_meta"] = map[string]any{"grokShell": true, "agentVersion": "1.0.25"}
			}
			sendResult(conn, msg, response)
		case "session/load", "session/resume":
			supported := os.Getenv("JAZ_FAKE_ACP_LOAD") != "0"
			if msg.Method == "session/resume" {
				supported = os.Getenv("JAZ_FAKE_ACP_RESUME") == "1"
			}
			if !supported {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.MethodNotFound(msg.Method))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			var req struct {
				Meta       map[string]any    `json:"_meta"`
				Cwd        string            `json:"cwd"`
				SessionID  string            `json:"sessionId"`
				MCPServers []json.RawMessage `json:"mcpServers"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil || req.SessionID != fakeSessionID {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("unknown session", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if path := os.Getenv("JAZ_FAKE_ACP_MATERIALIZED_SESSION"); path != "" {
				if _, err := os.Stat(path); err != nil {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, &jsonrpc.Error{Code: -32002, Message: "Resource not found"})
					_ = conn.Send(context.Background(), resp)
					continue
				}
			}
			if err := validateFakeCwdPrompt(req.Cwd, req.Meta); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if err := validateFakeRulesPrompt(req.Meta); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if err := validateFakeMCPServers(req.MCPServers); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if msg.Method == "session/load" {
				// History replay arrives before the load result resolves.
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"messageId":     "fake:replay:1",
						"content":       map[string]any{"type": "text", "text": "replayed history"},
					},
				})
			}
			result := map[string]any{
				"modes": fakeModes(),
			}
			addFakeModels(result, currentModel, codex)
			startedModel, lastTurnModel = currentModel, ""
			addFakePlanConfig(result)
			sendResult(conn, msg, result)
		case "session/new":
			var req struct {
				Meta       map[string]any    `json:"_meta"`
				Cwd        string            `json:"cwd"`
				MCPServers []json.RawMessage `json:"mcpServers"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("invalid session/new params", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if err := validateFakeMCPServers(req.MCPServers); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if want := os.Getenv("JAZ_FAKE_ACP_SYSTEM_PROMPT"); want != "" && req.Meta["systemPrompt"] != want {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing system prompt", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if err := validateFakeRulesPrompt(req.Meta); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if err := validateFakeCwdPrompt(req.Cwd, req.Meta); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if os.Getenv("JAZ_FAKE_ACP_EXPECT_ULTRACODE") == "1" && !fakeUltracodeMeta(req.Meta) {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing ultracode setting", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			result := map[string]any{
				"sessionId": fakeSessionID,
			}
			if modes := fakeModes(); modes != nil {
				result["modes"] = modes
			}
			addFakeModels(result, currentModel, codex)
			startedModel, lastTurnModel = currentModel, ""
			// codex-acp describes the efforts of the model the thread started on.
			if effort := fakeModelEffortOption(); codex && effort != nil {
				options, _ := result["configOptions"].([]map[string]any)
				result["configOptions"] = append(options, effort)
			}
			addFakePlanConfig(result)
			sendResult(conn, msg, result)
		case "session/close":
			sendResult(conn, msg, map[string]any{})
		case "session/set_mode":
			var req struct {
				ModeID string `json:"modeId"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil || !fakeModeSupported(req.ModeID) {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected supported mode", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			// Plan mode waits while a hold file sits in the agent's directory.
			if req.ModeID == "plan" {
				fakeHold(os.Getenv("JAZ_FAKE_ACP_PLAN_HOLD"))
			}
			currentMode = req.ModeID
			sendResult(conn, msg, map[string]any{})
		case "session/set_model":
			if os.Getenv("JAZ_FAKE_ACP_SET_MODEL") != "1" {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.MethodNotFound(msg.Method))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			var req struct {
				SessionID string `json:"sessionId"`
				ModelID   string `json:"modelId"`
			}
			want := os.Getenv("JAZ_FAKE_ACP_EXPECT_MODEL")
			if err := json.Unmarshal(msg.Params, &req); err != nil || req.SessionID != fakeSessionID || (want != "" && req.ModelID != want) {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected configured model", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			currentModel = req.ModelID
			sendResult(conn, msg, map[string]any{})
		case "session/set_config_option":
			if os.Getenv("JAZ_FAKE_ACP_SET_CONFIG") != "1" {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.MethodNotFound(msg.Method))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			var req struct {
				SessionID string `json:"sessionId"`
				ConfigID  string `json:"configId"`
				Value     string `json:"value"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil || req.SessionID != fakeSessionID {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected configured option", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if req.ConfigID == "collaboration_mode" {
				if req.Value != "default" && req.Value != "plan" {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected collaboration mode", nil))
					_ = conn.Send(context.Background(), resp)
					continue
				}
				currentCollaborationMode = req.Value
				sendResult(conn, msg, map[string]any{})
				continue
			}
			wantConfigID := os.Getenv("JAZ_FAKE_ACP_EXPECT_CONFIG_ID")
			if wantConfigID == "" {
				wantConfigID = "reasoning_effort"
			}
			want := os.Getenv("JAZ_FAKE_ACP_EXPECT_EFFORT")
			if req.ConfigID == "model" {
				wantModel := os.Getenv("JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG")
				if wantModel != "" && req.Value != wantModel {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected configured model option", nil))
					_ = conn.Send(context.Background(), resp)
					continue
				}
				currentModel = req.Value
				result := map[string]any{}
				if os.Getenv("JAZ_FAKE_ACP_MODEL_CONFIG_OMITS_EFFORT") == "1" {
					result["configOptions"] = []map[string]any{
						{"id": "mode", "type": "select", "options": []map[string]any{{"value": "auto"}}},
						{"id": "model", "type": "select", "options": []map[string]any{{"value": req.Value}}},
					}
				} else if effort := fakeModelEffortOption(); effort != nil {
					result["configOptions"] = []map[string]any{
						{"id": "model", "category": "model", "type": "select", "options": []map[string]any{{"value": req.Value}}},
						effort,
					}
				}
				sendResult(conn, msg, result)
				continue
			}
			if req.ConfigID != wantConfigID ||
				(want != "" && req.Value != want) {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected configured reasoning effort", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			currentEffort = req.Value
			sendResult(conn, msg, map[string]any{})
		case "_x.ai/interject":
			grokInterjections++
			pendingPrompt = fakeGrokInterject(t, conn, msg, pendingPrompt, grokInterjections)
		case "_session/steering":
			var req struct {
				WaitForCompletion bool `json:"waitForCompletion"`
			}
			if json.Unmarshal(msg.Params, &req) != nil || !req.WaitForCompletion || pendingPrompt == nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected completion-aware native steer", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			notify(conn, "session/update", map[string]any{
				"sessionId": fakeSessionID,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"messageId":     nextMessageID("native-steer"),
					"content":       map[string]any{"type": "text", "text": "hello from native steer"},
				},
			})
			sendResult(conn, pendingPrompt, map[string]any{"stopReason": "end_turn"})
			pendingPrompt = nil
			sendResult(conn, msg, map[string]any{"outcome": "injected", "stopReason": "end_turn"})
		case "session/prompt":
			if os.Getenv("JAZ_FAKE_ACP_MUSE_WRITE_CONFIG") == "1" {
				for _, name := range []string{"auth.json", "trust.json", ".auth.json.lock", ".trust.json.lock", ".settings.json.lock"} {
					if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "muse", name), []byte("native state"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if os.Getenv("JAZ_FAKE_ACP_PROMPT_DELAY") == "1" {
				time.Sleep(200 * time.Millisecond)
			}
			for _, offer := range fakeCommandOffer.FindAllStringSubmatch(string(msg.Params), -1) {
				commands := []map[string]any{{"name": "review", "description": "Review changes"}}
				if offer[1] == "offer" {
					commands = append(commands, map[string]any{"name": "compact", "description": "Compact the conversation"})
				}
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update":    map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": commands},
				})
			}
			for _, fill := range fakeContextFill.FindAllStringSubmatch(string(msg.Params), -1) {
				used, _ := strconv.Atoi(fill[1])
				size, _ := strconv.Atoi(fill[2])
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update":    map[string]any{"sessionUpdate": "usage_update", "used": used, "size": size},
				})
			}
			if path := os.Getenv("JAZ_FAKE_ACP_RELEASE_FILE"); path != "" && strings.Contains(string(msg.Params), cmp.Or(os.Getenv("JAZ_FAKE_ACP_HOLD_TEXT"), "until released")) {
				// The turn works until the test creates the release file, then
				// ends as usual.
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update":    map[string]any{"sessionUpdate": "tool_call", "toolCallId": "tool-held", "title": "held work", "status": "in_progress"},
				})
				for _, err := os.Stat(path); err != nil; _, err = os.Stat(path) {
					time.Sleep(10 * time.Millisecond)
				}
			}
			var promptReq struct {
				Meta map[string]any `json:"_meta"`
			}
			_ = json.Unmarshal(msg.Params, &promptReq)
			if authRequired {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InternalError("Authentication required", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if pendingPrompt != nil && os.Getenv("JAZ_FAKE_ACP_PROMPT_QUEUEING") == "1" {
				sendResult(conn, pendingPrompt, map[string]any{"stopReason": "end_turn"})
				pendingPrompt = nil
			}
			if want := os.Getenv("JAZ_FAKE_ACP_EXPECT_MODEL"); want != "" && currentModel != want {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("configured model was not set", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if strings.Contains(string(msg.Params), "break transport") {
				_, _ = fmt.Fprintln(os.Stdout, "not-json")
				os.Exit(0)
			}
			if strings.Contains(string(msg.Params), "close transport") {
				os.Exit(0)
			}
			if strings.Contains(string(msg.Params), "crash transport") {
				_, _ = fmt.Fprintln(os.Stderr, "fake runtime crash")
				os.Exit(7)
			}
			if err := validateFakePromptContains(msg.Params); err != nil {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams(err.Error(), nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if want := os.Getenv("JAZ_FAKE_ACP_EXPECT_EFFORT"); want != "" && currentEffort != want {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("configured reasoning effort was not set", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			// A turn on another model than the thread started on, or than the
			// previous turn ran, is one Codex announces to the model as a switch.
			logFakeTurn(t, currentModel, currentEffort, currentModel != cmp.Or(lastTurnModel, startedModel))
			lastTurnModel = currentModel
			if path := os.Getenv("JAZ_FAKE_ACP_MATERIALIZED_SESSION"); path != "" {
				if err := os.WriteFile(path, []byte(fakeSessionID), 0o600); err != nil {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InternalError(err.Error(), nil))
					_ = conn.Send(context.Background(), resp)
					continue
				}
			}
			for _, report := range fakeSubagentReport.FindAllStringSubmatch(string(msg.Params), -1) {
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update": map[string]any{
						"sessionUpdate": "session_info_update",
						"_meta": map[string]any{"jaz": map[string]any{"providerSubagent": map[string]any{
							"provider": "claude", "id": report[1], "status": report[2],
						}}},
					},
				})
			}
			if _, ok := fakeSideChatMeta(promptReq.Meta); ok {
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"_meta":         promptReq.Meta,
						"messageId":     nextMessageID("side-chat"),
						"content":       map[string]any{"type": "text", "text": "hello from side chat"},
					},
				})
				sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
				continue
			}
			if strings.Contains(string(msg.Params), "ask then block") {
				fakeAskThenBlock(conn, msg)
				continue
			}
			if strings.Contains(string(msg.Params), "block until cancelled") || os.Getenv("JAZ_FAKE_ACP_BLOCK_PROMPT") == "1" {
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update": map[string]any{
						"sessionUpdate": "tool_call",
						"toolCallId":    "tool-slow",
						"title":         "long running tool",
						"status":        "in_progress",
					},
				})
				if cancelArrived {
					cancelArrived = false
					sendResult(conn, msg, map[string]any{"stopReason": fakeCancelStopReason()})
					continue
				}
				pendingPrompt = msg
				continue
			}
			if currentMode == "plan" || currentCollaborationMode == "plan" {
				notify(conn, "session/update", map[string]any{
					"sessionId": fakeSessionID,
					"update": map[string]any{
						"sessionUpdate": "plan",
						"entries": []map[string]any{
							{"content": "Inspect request", "priority": "high", "status": "completed"},
							{"content": "Wait for approval", "priority": "medium", "status": "in_progress"},
						},
					},
				})
				sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
				continue
			}
			notify(conn, "session/update", map[string]any{
				"sessionId": fakeSessionID,
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"messageId":     nextMessageID("message"),
					"content":       map[string]any{"type": "text", "text": "hello from fake agent"},
				},
			})
			notify(conn, "session/update", map[string]any{
				"sessionId": fakeSessionID,
				"update": map[string]any{
					"sessionUpdate": "tool_call",
					"toolCallId":    "tool-1",
					"title":         "whoami",
					"status":        "completed",
				},
			})
			sendResult(conn, msg, map[string]any{"stopReason": "end_turn"})
		default:
			resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.MethodNotFound(msg.Method))
			_ = conn.Send(context.Background(), resp)
		}
	}
}

func logFakeACPRequest(t *testing.T, msg *jsonrpc.Message) {
	path := os.Getenv("JAZ_FAKE_ACP_REQUEST_LOG")
	if path == "" {
		return
	}
	entry, err := json.Marshal(struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}{Method: msg.Method, Params: msg.Params})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, string(entry)); err != nil {
		t.Fatal(err)
	}
}

func fakeCancelStopReason() string {
	if os.Getenv("JAZ_FAKE_ACP_CANCEL_END_TURN") == "1" {
		return "end_turn"
	}
	return "cancelled"
}

func validateFakePromptContains(params json.RawMessage) error {
	want := os.Getenv("JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS")
	if want == "" {
		return nil
	}
	raw := string(params)
	trigger := os.Getenv("JAZ_FAKE_ACP_EXPECT_PROMPT_TRIGGER")
	if trigger != "" && !strings.Contains(raw, trigger) {
		return nil
	}
	if !strings.Contains(raw, want) {
		return fmt.Errorf("prompt missing %q", want)
	}
	return nil
}

func validateFakeCwdPrompt(cwd string, meta map[string]any) error {
	if os.Getenv("JAZ_FAKE_ACP_EXPECT_CWD_IN_PROMPT") != "1" {
		return nil
	}
	prompt, _ := meta["systemPrompt"].(string)
	if cwd == "" || !strings.Contains(prompt, cwd) {
		return fmt.Errorf("system prompt missing cwd %q", cwd)
	}
	return nil
}

func validateFakeRulesPrompt(meta map[string]any) error {
	want := os.Getenv("JAZ_FAKE_ACP_RULES_CONTAINS")
	if want == "" {
		return nil
	}
	rules, _ := meta["rules"].(string)
	if !strings.Contains(rules, want) {
		return fmt.Errorf("missing grok rules prompt")
	}
	return nil
}

func fakeModes() map[string]any {
	if os.Getenv("JAZ_FAKE_ACP_NO_MODES") == "1" {
		return nil
	}
	currentMode := os.Getenv("JAZ_FAKE_ACP_REPORTED_MODE")
	if currentMode == "" {
		currentMode = os.Getenv("JAZ_FAKE_ACP_CURRENT_MODE")
	}
	if currentMode == "" {
		currentMode = "auto"
	}
	if os.Getenv("JAZ_FAKE_ACP_CLAUDE_MODES") == "1" {
		return map[string]any{
			"currentModeId": currentMode,
			"availableModes": []map[string]any{
				{"id": "auto", "name": "Auto"},
				{"id": "bypassPermissions", "name": "Bypass Permissions"},
				{"id": "acceptEdits", "name": "Accept Edits"},
				{"id": "plan", "name": "Plan"},
			},
		}
	}
	if os.Getenv("JAZ_FAKE_ACP_CODEX_MODES") == "1" {
		return map[string]any{
			"currentModeId": currentMode,
			"availableModes": []map[string]any{
				{"id": "agent", "name": "Agent"},
				{"id": "agent-full-access", "name": "Agent (full access)"},
			},
		}
	}
	return map[string]any{
		"currentModeId": currentMode,
		"availableModes": []map[string]any{
			{"id": "auto", "name": "Auto"},
			{"id": "full-access", "name": "Full Access"},
			{"id": "plan", "name": "Plan"},
		},
	}
}

func fakeModeSupported(mode string) bool {
	for _, id := range []string{"agent-full-access", "full-access", "always-approve", "bypassPermissions", "acceptEdits", "auto", "plan"} {
		if mode == id {
			return true
		}
	}
	return false
}

func fakeAskThenBlock(conn jsonrpc.MessageConn, prompt *jsonrpc.Message) {
	req, err := jsonrpc.NewRequest(json.RawMessage(`"fake-elicit-1"`), "elicitation/create", map[string]any{
		"mode":      "form",
		"sessionId": fakeSessionID,
		"message":   "Which workspace?",
		"requestedSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"choice": map[string]any{"type": "string", "title": "Choice"},
			},
		},
	})
	if err != nil {
		return
	}
	_ = conn.Send(context.Background(), req)

	messageSeq := 0
	finishSteered := func(steered *jsonrpc.Message) {
		messageSeq++
		notify(conn, "session/update", map[string]any{
			"sessionId": fakeSessionID,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"messageId":     fmt.Sprintf("fake:steered:%d", messageSeq),
				"content":       map[string]any{"type": "text", "text": "hello from fake agent"},
			},
		})
		sendResult(conn, steered, map[string]any{"stopReason": "end_turn"})
	}

	var steered *jsonrpc.Message
	parkedResolved := false
	for {
		next, err := conn.Receive(context.Background())
		if err != nil {
			os.Exit(0)
		}
		switch {
		case next.Method == "session/cancel":
			sendResult(conn, prompt, map[string]any{"stopReason": fakeCancelStopReason()})
			return
		case next.IsResponse():
			stop := "end_turn"
			if os.Getenv("JAZ_FAKE_ACP_ELICIT_CANCEL_STOP") == "1" {
				stop = "cancelled"
			}
			sendResult(conn, prompt, map[string]any{"stopReason": stop})
			parkedResolved = true
			if steered != nil {
				finishSteered(steered)
				return
			}
		case next.Method == "session/prompt":
			if !parkedResolved && os.Getenv("JAZ_FAKE_ACP_STRICT_ELICIT_HOL") == "1" {
				resp, _ := jsonrpc.NewErrorResponse(*next.ID, jsonrpc.InvalidParams("steered prompt arrived before elicitation response", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			steered = next
			if parkedResolved {
				finishSteered(steered)
				return
			}
		}
	}
}

func sendResult(conn jsonrpc.MessageConn, req *jsonrpc.Message, result any) {
	if raw := os.Getenv("JAZ_FAKE_ACP_SESSION_OPTIONS"); raw != "" && (req.Method == "session/new" || req.Method == "session/load" || req.Method == "session/resume" || req.Method == "session/set_config_option") {
		var options []map[string]any
		if err := json.Unmarshal([]byte(raw), &options); err != nil {
			panic(err)
		}
		var selection struct {
			ID    string `json:"configId"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(req.Params, &selection); err != nil {
			panic(err)
		}
		for _, option := range options {
			if option["id"] == selection.ID {
				option["currentValue"] = selection.Value
			}
		}
		result.(map[string]any)["configOptions"] = options
	}
	resp, err := jsonrpc.NewResult(*req.ID, result)
	if err == nil {
		_ = conn.Send(context.Background(), resp)
	}
}

func notify(conn jsonrpc.MessageConn, method string, params any) {
	if _, err := json.Marshal(params); err != nil {
		return
	}
	msg, err := jsonrpc.NewNotification(method, params)
	if err == nil {
		_ = conn.Send(context.Background(), msg)
	}
}

// addFakeModels advertises the agent's models as codex-acp does, with Codex's
// default as the model option's recommended value.
func addFakeModels(result map[string]any, currentModel string, codex bool) {
	raw := os.Getenv("JAZ_FAKE_ACP_MODELS")
	if strings.TrimSpace(raw) == "" {
		return
	}
	var available []map[string]any
	baseModels := make(map[string]struct{})
	for _, model := range strings.Split(raw, ",") {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		available = append(available, map[string]any{"modelId": model, "name": model})
		if i := strings.LastIndex(model, "/"); i > 0 {
			baseModels[model[:i]] = struct{}{}
		} else {
			baseModels[model] = struct{}{}
		}
	}
	var modelOptions []map[string]any
	for model := range baseModels {
		modelOptions = append(modelOptions, map[string]any{"value": model, "name": model})
	}
	result["models"] = map[string]any{"availableModels": available}
	option := map[string]any{
		"id":           "model",
		"name":         "Model",
		"type":         "select",
		"currentValue": currentModel,
		"options":      modelOptions,
	}
	if codex {
		option["_meta"] = map[string]any{"jetbrains": map[string]any{"air": map[string]any{"recommendedValue": fakeDefaultModel()}}}
	}
	result["configOptions"] = []map[string]any{option}
}

func addFakePlanConfig(result map[string]any) {
	if os.Getenv("JAZ_FAKE_ACP_PLAN_CONFIG") != "1" {
		return
	}
	options, _ := result["configOptions"].([]map[string]any)
	result["configOptions"] = append(options, map[string]any{
		"id":           "collaboration_mode",
		"name":         "Collaboration mode",
		"category":     "collaboration_mode",
		"type":         "select",
		"currentValue": "default",
		"options": []map[string]any{
			{"value": "default"},
			{"value": "plan"},
		},
	})
}

func fakeUltracodeMeta(meta map[string]any) bool {
	claudeCode, ok := meta["claudeCode"].(map[string]any)
	if !ok {
		return false
	}
	options, ok := claudeCode["options"].(map[string]any)
	if !ok {
		return false
	}
	settings, ok := options["settings"].(map[string]any)
	if !ok {
		return false
	}
	value, ok := settings["ultracode"].(bool)
	return ok && value
}

func fakeSideChatMeta(meta map[string]any) (map[string]any, bool) {
	codex, ok := meta["codex"].(map[string]any)
	if !ok {
		return nil, false
	}
	sideChat, ok := codex["sideChat"].(map[string]any)
	if !ok {
		return nil, false
	}
	_, ok = sideChat["id"].(string)
	return sideChat, ok
}

func validateFakeMCPServers(rawServers []json.RawMessage) error {
	if os.Getenv("JAZ_FAKE_ACP_EXPECT_MCP") != "1" {
		if len(rawServers) != 0 {
			return fmt.Errorf("unexpected mcp servers")
		}
		return nil
	}
	if len(rawServers) != 2 {
		return fmt.Errorf("mcp server count = %d, want 2", len(rawServers))
	}
	return validateFakeProxyAndJaztools(rawServers)
}

func validateFakeProxyAndJaztools(rawServers []json.RawMessage) error {
	servers := map[string]struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	}{}
	for _, raw := range rawServers {
		var server struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			URL     string `json:"url"`
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		}
		if err := json.Unmarshal(raw, &server); err != nil {
			return err
		}
		servers[server.Name] = server
	}
	proxy, ok := servers["jaz_mcp"]
	if !ok || proxy.Type != "http" || proxy.URL != "http://127.0.0.1:5299/mcp/proxy" {
		return fmt.Errorf("unexpected mcp proxy %#v", proxy)
	}
	if len(proxy.Headers) != 1 ||
		!strings.EqualFold(proxy.Headers[0].Name, mcpsession.HeaderName) ||
		strings.TrimSpace(proxy.Headers[0].Value) == "" {
		return fmt.Errorf("proxy must expose only a resolved session header, got %#v", proxy.Headers)
	}
	jaz, ok := servers["jaztools"]
	if !ok || jaz.Type != "http" || jaz.URL != "http://127.0.0.1:5299/mcp/jaztools" {
		return fmt.Errorf("unexpected jaztools server %#v", jaz)
	}
	if len(jaz.Headers) != 1 ||
		!strings.EqualFold(jaz.Headers[0].Name, mcpsession.HeaderName) ||
		strings.TrimSpace(jaz.Headers[0].Value) == "" {
		return fmt.Errorf("jaztools must expose only a resolved session header, got %#v", jaz.Headers)
	}
	return nil
}

// fakeHold waits while the file hold exists, and exits once Jaz is gone.
func fakeHold(hold string) {
	if hold == "" {
		return
	}
	for {
		if _, err := os.Stat(hold); err != nil {
			return
		}
		if os.Getppid() == 1 {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
