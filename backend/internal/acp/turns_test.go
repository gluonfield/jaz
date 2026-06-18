package acp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

type mutablePromptSource struct {
	prompt string
	err    error
	calls  int
}

func (s *mutablePromptSource) SkillsPrompt() (string, error) {
	s.calls++
	return s.prompt, s.err
}

func (s *mutablePromptSource) ACPPrompt(string) (string, error) { return s.prompt, s.err }

func TestACPTurnPromptContextRefreshesSkillsForMentions(t *testing.T) {
	source := &mutablePromptSource{prompt: "old skills"}
	manager := &Manager{cfg: Config{SystemPrompt: source}}

	if context, err := manager.turnPromptContext("plain request"); err != nil || context != "" {
		t.Fatalf("turnPromptContext without mention = %q, %v", context, err)
	}
	if source.calls != 0 {
		t.Fatalf("SkillsPrompt called without $ mention")
	}

	source.prompt = "latest skills"
	context, err := manager.turnPromptContext("use $new-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "latest skills") || !strings.Contains(context, "$skill references") {
		t.Fatalf("context did not include refreshed skills prompt:\n%s", context)
	}
	if source.calls != 1 {
		t.Fatalf("SkillsPrompt calls = %d, want 1", source.calls)
	}
}

func TestACPTurnPromptContextReturnsSkillPromptErrors(t *testing.T) {
	want := errors.New("boom")
	manager := &Manager{cfg: Config{SystemPrompt: &mutablePromptSource{err: want}}}

	_, err := manager.turnPromptContext("use $new-skill")
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestPromptContentBlocksPrependsContext(t *testing.T) {
	blocks, err := promptContentBlocks("context", "message", nil, localAttachmentResources)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want context + message", len(blocks))
	}
	var got []string
	for _, block := range blocks {
		decoded, err := acpschema.DecodeContentBlock(block)
		if err != nil {
			t.Fatal(err)
		}
		text, ok := decoded.(acpschema.TextContentBlock)
		if !ok {
			t.Fatalf("block = %T, want text", decoded)
		}
		got = append(got, text.Text)
	}
	if got[0] != "context" || got[1] != "message" {
		t.Fatalf("text blocks = %#v", got)
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
