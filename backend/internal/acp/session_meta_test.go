package acp

import (
	"context"
	"testing"
)

func TestSessionLoadMetaDoesNotReappendCodexSystemPrompt(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("base prompt")}}
	got, err := manager.sessionLoadMeta(context.Background(), AgentCodex, AgentConfig{}, "", "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["systemPrompt"]; ok {
		t.Fatalf("Codex load reattached persisted system prompt: %#v", got)
	}
}

func TestSessionLoadMetaReattachesOtherAgentSystemPrompt(t *testing.T) {
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("base prompt")}}
	got, err := manager.sessionLoadMeta(context.Background(), AgentKimi, AgentConfig{}, "", "", "", []string{"run context"})
	if err != nil {
		t.Fatal(err)
	}
	if got["systemPrompt"] != "base prompt\n\nrun context" {
		t.Fatalf("Kimi load system prompt = %#v", got)
	}
}
