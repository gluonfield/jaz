package messagepayload

import "testing"

func TestImplicitCanvasPageDoesNotStartAMessage(t *testing.T) {
	location := []MessageContext{{Type: ContextTypeCanvas, Page: "layout", Text: "Design"}}
	if HasMessageContent(" ", location, nil) {
		t.Fatal("the active page alone must not start a message")
	}
	if !HasMessageContent("add here", location, nil) || !HasMessageContent("", location, []Attachment{{ID: "drawing"}}) {
		t.Fatal("a request or attachment on the active page must remain sendable")
	}
	if !HasMessageContent("", []MessageContext{{Type: ContextTypeCanvas, Page: "layout", ID: "part"}}, nil) {
		t.Fatal("an explicitly selected item must remain sendable")
	}
}
