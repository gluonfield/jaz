package connections

import (
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/emailclean"
)

func gmailToolThread(content gmailconnector.ThreadContent) GmailThreadContent {
	messages := make([]GmailMessageContent, 0, len(content.Messages))
	for _, message := range content.Messages {
		messages = append(messages, gmailToolContent(message))
	}
	return GmailThreadContent{
		ID:       content.ID,
		Snippet:  emailclean.Text(content.Snippet),
		Messages: messages,
	}
}

func gmailToolContent(content gmailconnector.MessageContent) GmailMessageContent {
	text, textTruncated := clampGmailBody(emailclean.Body(content.BodyText, content.BodyHTML))
	return GmailMessageContent{
		Message:           gmailToolMessage(content.Message),
		BodyText:          text,
		BodyTextTruncated: textTruncated,
	}
}

func gmailToolThreadSummaries(threads []gmailconnector.Thread) []gmailconnector.Thread {
	out := make([]gmailconnector.Thread, 0, len(threads))
	for _, thread := range threads {
		messages := make([]gmailconnector.Message, 0, len(thread.Messages))
		for _, message := range thread.Messages {
			messages = append(messages, gmailToolMessage(message))
		}
		thread.HistoryID = ""
		thread.Snippet = emailclean.Text(thread.Snippet)
		thread.Messages = messages
		out = append(out, thread)
	}
	return out
}

func gmailToolDraft(draft gmailconnector.Draft) gmailconnector.Draft {
	draft.Message = gmailToolMessage(draft.Message)
	return draft
}

func gmailToolDrafts(drafts []gmailconnector.Draft) []gmailconnector.Draft {
	out := make([]gmailconnector.Draft, 0, len(drafts))
	for _, draft := range drafts {
		out = append(out, gmailToolDraft(draft))
	}
	return out
}

func gmailToolMessage(message gmailconnector.Message) gmailconnector.Message {
	message.HistoryID = ""
	message.MessageID = ""
	message.References = ""
	message.InReplyTo = ""
	message.Snippet = emailclean.Text(message.Snippet)
	return message
}

func clampGmailBody(body string) (string, bool) {
	if body == "" {
		return "", false
	}
	runes := []rune(body)
	if len(runes) <= maxGmailBodyChars {
		return body, false
	}
	return string(runes[:maxGmailBodyChars]) + "...", true
}
