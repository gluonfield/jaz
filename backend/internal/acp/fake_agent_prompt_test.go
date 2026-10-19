package acp_test

import (
	"os"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

func fakePromptBoundary(t *testing.T, conn jsonrpc.MessageConn) {
	if path := os.Getenv("JAZ_FAKE_ACP_EFFECT"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString("effect\n"); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	kind := os.Getenv("JAZ_FAKE_ACP_FIRST_UPDATE")
	var update map[string]any
	switch kind {
	case "chunk", "thought":
		name := "agent_message_chunk"
		if kind == "thought" {
			name = "agent_thought_chunk"
		}
		update = map[string]any{"sessionUpdate": name, "content": map[string]any{"type": "text", "text": "starting"}}
	case "tool":
		update = map[string]any{"sessionUpdate": "tool_call", "toolCallId": "first-tool", "title": "work", "status": "in_progress"}
	case "plan":
		update = map[string]any{"sessionUpdate": "plan", "entries": []map[string]any{{"content": "work", "status": "in_progress", "priority": "high"}}}
	case "metadata":
		update = map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": []map[string]any{{"name": "review"}}}
	}
	if update != nil {
		notify(conn, "session/update", map[string]any{"sessionId": fakeSessionID, "update": update})
	}
	if path := os.Getenv("JAZ_FAKE_ACP_BOUNDARY_READY"); path != "" {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
