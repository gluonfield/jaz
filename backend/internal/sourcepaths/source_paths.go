package sourcepaths

import (
	"path"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func ChatContactPath(provider, account string) string {
	return path.Join(chatProviderRoot(provider), account, "contacts.md")
}

func ChatConversationsPrefix(provider, account string) string {
	return chatConversationsDir(provider, account) + "/"
}

func ChatConversationDayPath(provider, account, conversation string, occurred time.Time) string {
	utc := occurred.UTC()
	return path.Join(chatConversationsDir(provider, account), integrations.SourceSlug(conversation), utc.Format("2006"), utc.Format("01"), utc.Format("02")+".md")
}

func ChatConversationSegmentsDayPath(provider, account string, occurred time.Time, segments ...string) string {
	utc := occurred.UTC()
	parts := append([]string{chatConversationsDir(provider, account)}, segments...)
	parts = append(parts, utc.Format("2006"), utc.Format("01"), utc.Format("02")+".md")
	return path.Join(parts...)
}

func EmailMessagesPrefix(provider, account string) string {
	return emailMessagesDir(provider, account) + "/"
}

func EmailMessagePath(provider, account string, occurred time.Time, messageID string) string {
	utc := occurred.UTC()
	return path.Join(emailMessagesDir(provider, account), utc.Format("2006"), utc.Format("01"), utc.Format("02"), integrations.SourceSlug(messageID)+".md")
}

func chatConversationsDir(provider, account string) string {
	return path.Join(chatProviderRoot(provider), account, "conversations")
}

func emailMessagesDir(provider, account string) string {
	return path.Join(emailProviderRoot(provider), account, "messages")
}

func chatProviderRoot(provider string) string {
	return path.Join("sources", "chat", provider)
}

func emailProviderRoot(provider string) string {
	return path.Join("sources", "email", provider)
}
