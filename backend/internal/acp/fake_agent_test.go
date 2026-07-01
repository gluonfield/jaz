package acp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

func TestFakeACPAgentProcess(t *testing.T) {
	if os.Getenv("JAZ_FAKE_ACP_AGENT") != "1" {
		return
	}
	if msg := os.Getenv("JAZ_FAKE_ACP_EXIT_BEFORE_INIT"); msg != "" {
		_, _ = fmt.Fprintln(os.Stderr, msg)
		os.Exit(2)
	}
	conn := stdio.New(os.Stdin, os.Stdout)
	currentMode := os.Getenv("JAZ_FAKE_ACP_CURRENT_MODE")
	if currentMode == "" {
		currentMode = "auto"
	}
	currentModel := ""
	currentEffort := ""
	var pendingPrompt *jsonrpc.Message
	cancelArrived := false
	for {
		msg, err := conn.Receive(context.Background())
		if err != nil {
			os.Exit(0)
		}
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
				if err := json.Unmarshal(msg.Params, &req); err != nil || req.ClientCapabilities.Meta["terminal-auth"] != true {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing terminal auth capability", nil))
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
				"loadSession": os.Getenv("JAZ_FAKE_ACP_LOAD") == "1",
			}
			if os.Getenv("JAZ_FAKE_ACP_MCP_HTTP") == "1" {
				capabilities["mcpCapabilities"] = map[string]any{"http": true}
			}
			if os.Getenv("JAZ_FAKE_ACP_PROMPT_QUEUEING") == "1" {
				capabilities["_meta"] = map[string]any{
					"claudeCode": map[string]any{"promptQueueing": true},
				}
			}
			sendResult(conn, msg, map[string]any{
				"protocolVersion":   1,
				"agentInfo":         map[string]any{"name": "fake-agent", "version": "test"},
				"agentCapabilities": capabilities,
			})
		case "session/load":
			var req struct {
				Meta       map[string]any    `json:"_meta"`
				Cwd        string            `json:"cwd"`
				SessionID  string            `json:"sessionId"`
				MCPServers []json.RawMessage `json:"mcpServers"`
			}
			if err := json.Unmarshal(msg.Params, &req); err != nil || req.SessionID != "fake-session" {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("unknown session", nil))
				_ = conn.Send(context.Background(), resp)
				continue
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
			// History replay arrives before the load result resolves.
			notify(conn, "session/update", map[string]any{
				"sessionId": "fake-session",
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": "replayed history"},
				},
			})
			sendResult(conn, msg, map[string]any{
				"modes": fakeModes(),
			})
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
				"sessionId": "fake-session",
			}
			if modes := fakeModes(); modes != nil {
				result["modes"] = modes
			}
			addFakeModels(result)
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
			if err := json.Unmarshal(msg.Params, &req); err != nil || req.SessionID != "fake-session" || (want != "" && req.ModelID != want) {
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
			if err := json.Unmarshal(msg.Params, &req); err != nil || req.SessionID != "fake-session" {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("expected configured option", nil))
				_ = conn.Send(context.Background(), resp)
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
				} else if effortConfigID := strings.TrimSpace(os.Getenv("JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_ID")); effortConfigID != "" {
					effortOptions := []map[string]any{}
					rawEffortOptions := strings.TrimSpace(os.Getenv("JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_OPTIONS"))
					if rawEffortOptions == "" {
						rawEffortOptions = os.Getenv("JAZ_FAKE_ACP_EXPECT_EFFORT")
					}
					for _, value := range strings.Split(rawEffortOptions, ",") {
						if value = strings.TrimSpace(value); value != "" {
							effortOptions = append(effortOptions, map[string]any{"value": value})
						}
					}
					result["configOptions"] = []map[string]any{
						{"id": "model", "category": "model", "type": "select", "options": []map[string]any{{"value": req.Value}}},
						{"id": effortConfigID, "category": "thought_level", "type": "select", "options": effortOptions},
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
		case "session/prompt":
			var promptReq struct {
				Meta map[string]any `json:"_meta"`
			}
			_ = json.Unmarshal(msg.Params, &promptReq)
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
			if objective := os.Getenv("JAZ_FAKE_ACP_GOAL_OBJECTIVE"); objective != "" {
				notify(conn, "thread/goal/updated", map[string]any{
					"threadId": "fake-session",
					"goal": map[string]any{
						"provider":        "codex",
						"providerGoalId":  "fake-goal-1",
						"objective":       objective,
						"status":          "active",
						"budgetSource":    "goal",
						"tokenBudget":     1000,
						"tokensUsed":      42,
						"remainingTokens": 958,
					},
				})
			}
			if _, ok := fakeSideChatMeta(promptReq.Meta); ok {
				notify(conn, "session/update", map[string]any{
					"sessionId": "fake-session",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"_meta":         promptReq.Meta,
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
			if strings.Contains(string(msg.Params), "block until cancelled") {
				notify(conn, "session/update", map[string]any{
					"sessionId": "fake-session",
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
			if currentMode == "plan" {
				notify(conn, "session/update", map[string]any{
					"sessionId": "fake-session",
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
				"sessionId": "fake-session",
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": "hello from fake agent"},
				},
			})
			notify(conn, "session/update", map[string]any{
				"sessionId": "fake-session",
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
	for _, id := range []string{"full-access", "always-approve", "bypassPermissions", "acceptEdits", "auto", "plan"} {
		if mode == id {
			return true
		}
	}
	return false
}

func fakeAskThenBlock(conn jsonrpc.MessageConn, prompt *jsonrpc.Message) {
	req, err := jsonrpc.NewRequest(json.RawMessage(`"fake-elicit-1"`), "elicitation/create", map[string]any{
		"mode":      "form",
		"sessionId": "fake-session",
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

	finishSteered := func(steered *jsonrpc.Message) {
		notify(conn, "session/update", map[string]any{
			"sessionId": "fake-session",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
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

func addFakeModels(result map[string]any) {
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
	result["configOptions"] = []map[string]any{
		{
			"id":      "model",
			"name":    "Model",
			"type":    "select",
			"options": modelOptions,
		},
	}
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
