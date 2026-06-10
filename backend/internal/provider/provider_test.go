package provider

import "testing"

func TestMessageContentExtractsTextFromMultipartUserMessage(t *testing.T) {
	msg := UserMessageParts(
		TextPart("look at this"),
		ImageURLPart("data:image/png;base64,abc", "auto"),
	)
	if got := MessageContent(msg); got != "look at this" {
		t.Fatalf("MessageContent() = %q, want text part", got)
	}
}
