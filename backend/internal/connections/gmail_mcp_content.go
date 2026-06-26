package connections

import gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"

func gmailToolThread(content gmailconnector.ThreadContent) GmailThreadContent {
	messages := make([]GmailMessageContent, 0, len(content.Messages))
	for _, message := range content.Messages {
		messages = append(messages, gmailToolContent(message))
	}
	return GmailThreadContent{
		ID:        content.ID,
		HistoryID: content.HistoryID,
		Snippet:   content.Snippet,
		Messages:  messages,
	}
}

func gmailToolContent(content gmailconnector.MessageContent) GmailMessageContent {
	text, textTruncated := clampGmailBody(content.BodyText)
	html := ""
	htmlTruncated := false
	if text == "" {
		html, htmlTruncated = clampGmailBody(content.BodyHTML)
	}
	return GmailMessageContent{
		Message:           content.Message,
		BodyText:          text,
		BodyTextTruncated: textTruncated,
		BodyHTML:          html,
		BodyHTMLTruncated: htmlTruncated,
	}
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
