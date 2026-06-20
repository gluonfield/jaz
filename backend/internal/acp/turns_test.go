package acp

import (
	"encoding/json"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

func TestPromptContentBlocksKeepsSkillReferencesInUserMessage(t *testing.T) {
	message := "use [$thermo-nuclear-code-quality-review](/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md)"
	blocks, err := promptContentBlocks(message, nil, localAttachmentResources)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want user message only", len(blocks))
	}
	decoded, err := acpschema.DecodeContentBlock(blocks[0])
	if err != nil {
		t.Fatal(err)
	}
	text, ok := decoded.(acpschema.TextContentBlock)
	if !ok {
		t.Fatalf("block = %T, want text", decoded)
	}
	if text.Text != message {
		t.Fatalf("text = %q, want %q", text.Text, message)
	}
}

func TestMessageWithSelectionsLabelsQuotesAboveMessage(t *testing.T) {
	got := messageWithSelections("explain this", []string{"first quote", "  ", "second quote"})
	want := "<selected_text>\n" +
		"<selection n=\"1\">\nfirst quote\n</selection>\n" +
		"<selection n=\"2\">\nsecond quote\n</selection>\n" +
		"</selected_text>\n\n" +
		"explain this"
	if got != want {
		t.Fatalf("messageWithSelections() = %q, want %q", got, want)
	}
}

func TestMessageWithSelectionsNoQuotesIsUnchanged(t *testing.T) {
	if got := messageWithSelections("plain message", nil); got != "plain message" {
		t.Fatalf("messageWithSelections() = %q, want %q", got, "plain message")
	}
}

func TestACPTurnErrorMessageExtractsNestedProviderError(t *testing.T) {
	err := &jsonrpc.Error{
		Code:    -32603,
		Message: "Internal error",
		Data: json.RawMessage(`{
			"message":"{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The 'gpt-5.3-codex-spark' model is not supported when using Codex with a ChatGPT account.\"}}",
			"codex_error_info":"other"
		}`),
	}
	got := acpTurnErrorMessage(err)
	want := "The 'gpt-5.3-codex-spark' model is not supported when using Codex with a ChatGPT account."
	if got != want {
		t.Fatalf("acpTurnErrorMessage() = %q, want %q", got, want)
	}
}
