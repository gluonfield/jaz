package acp

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestKimiNativeCatalogProcess(t *testing.T) {
	root := os.Getenv("JAZ_TEST_KIMI_CATALOG")
	if root == "" {
		return
	}
	mode, _ := os.ReadFile(filepath.Join(root, "mode"))
	log, err := os.OpenFile(filepath.Join(root, "requests.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	model, effort := "native-z", "high"
	choices := []map[string]string{{"value": "native-z", "name": "Effective Z"}, {"value": "native-a", "name": "Native A"}, {"value": "native-plain", "name": "Plain"}}
	levels := map[string][]string{"native-z": {"low", "high", "max"}, "native-a": {"off", "on"}}
	if string(mode) == "empty" {
		model, choices = "", []map[string]string{}
	}
	response := func() map[string]any {
		options := []map[string]any{{"id": "model", "category": "model", "type": "select", "currentValue": model, "options": choices}}
		if values := slices.Clone(levels[model]); len(values) > 0 {
			if !slices.Contains(values, effort) {
				values = append(values, effort)
			}
			entries := make([]map[string]string, 0, len(values))
			for _, value := range values {
				entries = append(entries, map[string]string{"value": value, "name": value})
			}
			options = append(options, map[string]any{"id": "thinking", "category": "thought_level", "type": "select", "currentValue": effort, "options": entries})
		}
		return map[string]any{"configOptions": options}
	}
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	for {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := decoder.Decode(&req); err == io.EOF {
			os.Exit(0)
		} else if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(log).Encode(req); err != nil {
			t.Fatal(err)
		}
		result := map[string]any{}
		var failure map[string]any
		switch req.Method {
		case "initialize":
			capabilities := map[string]any{"loadSession": true}
			if string(mode) != "no-delete" {
				capabilities["sessionCapabilities"] = map[string]any{"delete": map[string]any{}}
			}
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": capabilities}
		case "session/new":
			if err := os.WriteFile(filepath.Join(root, "live"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if string(mode) == "hold-new" {
				for {
					if _, err := os.Stat(filepath.Join(root, "release")); err == nil {
						break
					}
					time.Sleep(time.Millisecond)
				}
				time.Sleep(50 * time.Millisecond)
			}
			result = response()
			result["sessionId"] = "catalog-probe"
			if string(mode) == "malformed-options" {
				result["configOptions"] = "invalid"
			}
		case "session/set_config_option":
			var params struct {
				ConfigID string `json:"configId"`
				Value    string `json:"value"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params.ConfigID == "model" {
				if string(mode) == "hold-switch" {
					if err := os.WriteFile(filepath.Join(root, "switching"), nil, 0o600); err != nil {
						t.Fatal(err)
					}
					continue
				}
				model = params.Value
			} else if params.ConfigID == "thinking" && slices.Contains(levels[model], params.Value) {
				effort = params.Value
			} else {
				failure = map[string]any{"code": -32602, "message": "unsupported native effort"}
			}
			if string(mode) == "fail-switch" {
				failure = map[string]any{"code": -32602, "message": "model unavailable"}
			}
			result = response()
		case "session/delete":
			if string(mode) == "fail-delete" {
				failure = map[string]any{"code": -32603, "message": "delete failed"}
			} else if err := os.Remove(filepath.Join(root, "live")); err != nil {
				t.Fatal(err)
			}
		default:
			failure = map[string]any{"code": -32601, "message": "unexpected method"}
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
		if failure != nil {
			reply = map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": failure}
		}
		if err := encoder.Encode(reply); err != nil {
			t.Fatal(err)
		}
	}
}
