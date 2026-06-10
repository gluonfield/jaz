package acp

import (
	"encoding/json"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

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
