package sourcepaths

import (
	"testing"
	"time"
)

func TestSourcePaths(t *testing.T) {
	when := time.Date(2026, 6, 27, 10, 42, 0, 0, time.FixedZone("BST", 3600))
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"chat contact", ChatContactPath("telegram", "42"), "sources/chat/telegram/42/contacts.md"},
		{"email messages prefix", EmailMessagesPrefix("gmail", "personal"), "sources/email/gmail/personal/messages/"},
		{"chat conversations prefix", ChatConversationsPrefix("whatsapp", "personal"), "sources/chat/whatsapp/personal/conversations/"},
		{"email message", EmailMessagePath("gmail", "personal", when, "msg/1"), "sources/email/gmail/personal/messages/2026/06/27/msg-1-588a29c4.md"},
		{"chat conversation day", ChatConversationDayPath("telegram", "42", "user/1", when), "sources/chat/telegram/42/conversations/user-1-a298c916/2026/06/27.md"},
		{"chat conversation segments day", ChatConversationSegmentsDayPath("telegram", "42", when, "user", "1"), "sources/chat/telegram/42/conversations/user/1/2026/06/27.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("path = %q, want %q", tc.got, tc.want)
			}
		})
	}
}
