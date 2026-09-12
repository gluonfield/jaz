package acp

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestVoiceContextSeparatesPromptFromUserMessage(t *testing.T) {
	request := "What files are here?"
	context := "User: Hello\nVoice: Hello. What can I help with?"
	prompt, contexts := promptMessageAndContexts(request, []storage.MessageContext{
		{Type: storage.ContextTypeVoice, ID: "call", RequestID: "handoff", Text: context},
	})
	if !strings.Contains(prompt, "<voice_conversation>\n") || !strings.Contains(prompt, context) || !strings.Contains(prompt, request) {
		t.Fatalf("agent prompt = %q", prompt)
	}
	record := storage.UserMessageRecord(request, contexts, nil)
	if record.Content != request {
		t.Fatalf("display message = %q", record.Content)
	}
	if len(record.Blocks) != 2 || record.Blocks[0].Type != storage.BlockTypeVoiceContext || record.Blocks[0].ID != "call" || record.Blocks[0].RequestID != "handoff" || record.Blocks[0].Text != context {
		t.Fatalf("stored context = %#v", record.Blocks)
	}
}
