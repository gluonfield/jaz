package acp

import (
	"context"
	"testing"
)

func TestSessionRestoreMetaDoesNotReappendCodexSystemPrompt(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("base prompt")}}
	got, err := manager.sessionRestoreMeta(context.Background(), AgentCodex, AgentConfig{}, "", "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["systemPrompt"]; ok {
		t.Fatalf("Codex load reattached persisted system prompt: %#v", got)
	}
}

func TestSessionRestoreMetaReattachesOtherAgentSystemPrompt(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("base prompt")}}
	got, err := manager.sessionRestoreMeta(context.Background(), AgentKimi, AgentConfig{}, "", "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != "base prompt\n\nrun context" {
		t.Fatalf("Kimi load system prompt = %#v", got)
	}
}
