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

func TestLiveCodexACPSideChatProbe(t *testing.T) {
	timeout := 120 * time.Second
	if raw := os.Getenv("ACP_PROBE_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	root := firstNonEmpty(os.Getenv("ACP_PROBE_ROOT"), filepath.Join(os.Getenv("HOME"), ".jaz"))
	cwd := firstNonEmpty(os.Getenv("ACP_PROBE_CWD"), filepath.Join(root, "tmp", "codex-sidechat-probe"))
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := probeAgentConfig(t, AgentCodex)
	manager := NewManager(nil, Config{Root: root, Workspace: cwd}, log.New(io.Discard))
	env := manager.processEnv(AgentCodex, cfg)
	applyProbeEnvOverrides(env)
	conn, cleanup := probeOpenConn(t, ctx, AgentCodex, cfg, env, cwd)
	defer cleanup()

	init := probeCall(t, ctx, conn, "1", acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
		ClientInfo: &acpschema.Implementation{
			Name:    "jaz-sidechat-probe",
			Title:   "Jaz Side Chat Probe",
			Version: "0.1.0",
		},
		ClientCapabilities: &acpschema.ClientCapabilities{
			Meta: map[string]any{"terminal-auth": true},
			FS:   &acpschema.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
		},
	})
	if methodID, missing := autoAuthMethod(AgentCodex, init.Result, env); methodID != "" {
		_ = probeCall(t, ctx, conn, "2", acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID})
	} else if len(missing) > 0 {
		t.Skipf("missing Codex auth: %s", strings.Join(missing, ", "))
	}

	session := probeNewSession(t, ctx, conn, "3", cwd)
	probeApplyConfiguredSessionOptions(t, ctx, conn, AgentCodex, session.SessionID)
	sideID := "side-probe"
	first := probeSidePrompt(t, ctx, conn, "4", session.SessionID, sideID, "Reply with exactly SIDECHAT_ONE.")
	firstThreadID := requireProbeSideMeta(t, first, sideID, string(session.SessionID))
	second := probeSidePrompt(t, ctx, conn, "5", session.SessionID, sideID, "Reply with exactly SIDECHAT_TWO.")
	secondThreadID := requireProbeSideMeta(t, second, sideID, string(session.SessionID))
	if secondThreadID != firstThreadID {
		t.Fatalf("side chat thread id changed: first=%q second=%q", firstThreadID, secondThreadID)
	}
}

func probeSidePrompt(t *testing.T, ctx context.Context, conn jsonrpc.MessageConn, id string, sessionID acpschema.SessionID, sideID, prompt string) []map[string]any {
	t.Helper()
	msg, err := jsonrpc.NewRequest(json.RawMessage(id), acpschema.AgentMethodSessionPrompt, map[string]any{
		"sessionId": sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": prompt},
		},
		"_meta": map[string]any{
			"codex": map[string]any{
				"sideChat": map[string]any{
					"id":              sideID,
					"command":         "side",
					"parentSessionId": string(sessionID),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	probeLogMessage(t, "client -> agent", msg)
	if err := conn.Send(ctx, msg); err != nil {
		t.Fatal(err)
	}

	var metas []map[string]any
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
		if msg.IsNotification() {
			if msg.Method == acpschema.ClientMethodSessionUpdate {
				if meta, ok := probeSideMeta(t, msg, sessionID); ok {
					metas = append(metas, meta)
				}
			}
			continue
		}
		if msg.IsResponse() && msg.ID != nil && string(*msg.ID) == id {
			if msg.Error != nil {
				t.Fatalf("%s failed: %v", acpschema.AgentMethodSessionPrompt, msg.Error)
			}
			var response acpschema.PromptResponse
			if err := json.Unmarshal(msg.Result, &response); err != nil {
				t.Fatal(err)
			}
			if response.StopReason != acpschema.StopReasonEndTurn {
				t.Fatalf("stop reason = %q, want %q", response.StopReason, acpschema.StopReasonEndTurn)
			}
			return metas
		}
	}
}

func probeSideMeta(t *testing.T, msg *jsonrpc.Message, sessionID acpschema.SessionID) (map[string]any, bool) {
	t.Helper()
	var notification struct {
		SessionID acpschema.SessionID `json:"sessionId"`
		Update    json.RawMessage     `json:"update"`
	}
	if err := json.Unmarshal(msg.Params, &notification); err != nil {
		t.Fatal(err)
	}
	if notification.SessionID != sessionID {
		return nil, false
	}
	var update struct {
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(notification.Update, &update); err != nil {
		t.Fatal(err)
	}
	codex, ok := update.Meta["codex"].(map[string]any)
	if !ok {
		return nil, false
	}
	side, ok := codex["sideChat"].(map[string]any)
	return side, ok
}

func requireProbeSideMeta(t *testing.T, metas []map[string]any, sideID, parentSessionID string) string {
	t.Helper()
	for _, meta := range metas {
		if meta["id"] != sideID || meta["parentSessionId"] != parentSessionID {
			continue
		}
		threadID, _ := meta["threadId"].(string)
		if threadID == "" {
			t.Fatalf("side chat meta missing threadId: %#v", meta)
		}
		return threadID
	}
	t.Fatalf("missing side chat metadata for id=%q parent=%q in %#v", sideID, parentSessionID, metas)
	return ""
}
