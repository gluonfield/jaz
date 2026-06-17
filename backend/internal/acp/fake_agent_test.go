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
					sendResult(conn, pendingPrompt, map[string]any{"stopReason": "cancelled"})
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
						Meta map[string]any `json:"_meta"`
					} `json:"clientCapabilities"`
				}
				if err := json.Unmarshal(msg.Params, &req); err != nil || req.ClientCapabilities.Meta["terminal-auth"] != true {
					resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("missing terminal auth capability", nil))
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
			if want := os.Getenv("JAZ_FAKE_ACP_EXPECT_MODEL"); want != "" && currentModel != want {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("configured model was not set", nil))
				_ = conn.Send(context.Background(), resp)
				continue
			}
			if strings.Contains(string(msg.Params), "break transport") {
				_, _ = fmt.Fprintln(os.Stdout, "not-json")
				os.Exit(0)
			}
			if want := os.Getenv("JAZ_FAKE_ACP_EXPECT_EFFORT"); want != "" && currentEffort != want {
				resp, _ := jsonrpc.NewErrorResponse(*msg.ID, jsonrpc.InvalidParams("configured reasoning effort was not set", nil))
				_ = conn.Send(context.Background(), resp)
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
					sendResult(conn, msg, map[string]any{"stopReason": "cancelled"})
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

func validateFakeMCPServers(rawServers []json.RawMessage) error {
	if os.Getenv("JAZ_FAKE_ACP_EXPECT_MCP") != "1" {
		if len(rawServers) != 0 {
			return fmt.Errorf("unexpected mcp servers")
		}
		return nil
	}
	if len(rawServers) != 1 {
		return fmt.Errorf("mcp server count = %d, want 1", len(rawServers))
	}
	var server struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(rawServers[0], &server); err != nil {
		return err
	}
	if server.Type != "http" || server.Name != "Remote Docs" || server.URL != "https://mcp.example.com/mcp" {
		return fmt.Errorf("unexpected mcp server %#v", server)
	}
	if server.Headers == nil {
		return fmt.Errorf("mcp headers must be an array")
	}
	headers := map[string]string{}
	for _, header := range server.Headers {
		headers[header.Name] = header.Value
	}
	if headers["X-Literal"] != "literal" || headers["X-Secret"] != "secret" {
		return fmt.Errorf("unexpected mcp headers %#v", headers)
	}
	return nil
}
