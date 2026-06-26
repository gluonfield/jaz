package gmail

import (
	"encoding/base64"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

type threadList struct {
	Threads            []threadRef `json:"threads"`
	NextPageToken      string      `json:"nextPageToken"`
	ResultSizeEstimate int64       `json:"resultSizeEstimate"`
}

type threadRef struct {
	ID string `json:"id"`
}

type messageList struct {
	Messages           []messageRef `json:"messages"`
	NextPageToken      string       `json:"nextPageToken"`
	ResultSizeEstimate int64        `json:"resultSizeEstimate"`
}

type messageRef struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type historyList struct {
	History       []historyEntry `json:"history"`
	NextPageToken string         `json:"nextPageToken"`
	HistoryID     string         `json:"historyId"`
}

type historyEntry struct {
	MessagesAdded []historyMessageAdded `json:"messagesAdded"`
}

type historyMessageAdded struct {
	Message messageRef `json:"message"`
}

type draftList struct {
	Drafts             []draftRef `json:"drafts"`
	NextPageToken      string     `json:"nextPageToken"`
	ResultSizeEstimate int64      `json:"resultSizeEstimate"`
}

type apiAttachment struct {
	Data string `json:"data"`
	Size int64  `json:"size"`
}

type draftRef struct {
	ID string `json:"id"`
}

type apiDraftRequest struct {
	ID      string        `json:"id,omitempty"`
	Message apiRawMessage `json:"message,omitempty"`
}

type apiRawMessage struct {
	Raw      string `json:"raw,omitempty"`
	ThreadID string `json:"threadId,omitempty"`
}

type apiDraft struct {
	ID      string     `json:"id"`
	Message apiMessage `json:"message"`
}

type apiThread struct {
	ID        string       `json:"id"`
	HistoryID string       `json:"historyId"`
	Snippet   string       `json:"snippet"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	ID           string      `json:"id"`
	ThreadID     string      `json:"threadId"`
	HistoryID    string      `json:"historyId"`
	Snippet      string      `json:"snippet"`
	LabelIDs     []string    `json:"labelIds"`
	InternalDate string      `json:"internalDate"`
	Payload      messagePart `json:"payload"`
}

type messagePart struct {
	MIMEType string          `json:"mimeType"`
	Filename string          `json:"filename"`
	Headers  []messageHeader `json:"headers"`
	Body     messageBody     `json:"body"`
	Parts    []messagePart   `json:"parts"`
}

type messageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type messageBody struct {
	Data         string `json:"data"`
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
}

func metadataHeaders() []string {
	return []string{"From", "To", "Cc", "Bcc", "Reply-To", "Subject", "Date", "Message-ID", "References", "In-Reply-To"}
}

func messageFromAPI(raw apiMessage) Message {
	headers := headersByName(raw.Payload.Headers)
	return Message{
		ID:           raw.ID,
		ThreadID:     raw.ThreadID,
		HistoryID:    raw.HistoryID,
		MessageID:    headers["message-id"],
		References:   headers["references"],
		InReplyTo:    headers["in-reply-to"],
		Subject:      headers["subject"],
		Snippet:      raw.Snippet,
		From:         parseAddresses(headers["from"]),
		ReplyTo:      parseAddresses(headers["reply-to"]),
		To:           parseAddresses(headers["to"]),
		Cc:           parseAddresses(headers["cc"]),
		Bcc:          parseAddresses(headers["bcc"]),
		LabelIDs:     raw.LabelIDs,
		InternalDate: internalDate(raw.InternalDate),
		Attachments:  attachments(raw.Payload),
	}
}

func messageContentFromAPI(raw apiMessage) MessageContent {
	text, html := messageBodies(raw.Payload)
	return MessageContent{
		Message:  messageFromAPI(raw),
		BodyText: text,
		BodyHTML: html,
	}
}

func threadFromAPI(raw apiThread) Thread {
	messages := make([]Message, 0, len(raw.Messages))
	for _, message := range raw.Messages {
		messages = append(messages, messageFromAPI(message))
	}
	return Thread{
		ID:        raw.ID,
		HistoryID: raw.HistoryID,
		Snippet:   raw.Snippet,
		Messages:  messages,
	}
}

func threadContentFromAPI(raw apiThread) ThreadContent {
	messages := make([]MessageContent, 0, len(raw.Messages))
	for _, message := range raw.Messages {
		messages = append(messages, messageContentFromAPI(message))
	}
	return ThreadContent{
		ID:        raw.ID,
		HistoryID: raw.HistoryID,
		Snippet:   raw.Snippet,
		Messages:  messages,
	}
}

func draftFromAPI(raw apiDraft) Draft {
	return Draft{
		ID:      raw.ID,
		Message: messageFromAPI(raw.Message),
	}
}

func headersByName(headers []messageHeader) map[string]string {
	out := map[string]string{}
	for _, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header.Name))
		value := strings.TrimSpace(header.Value)
		if name != "" && value != "" {
			out[name] = value
		}
	}
	return out
}

func parseAddresses(value string) []Address {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return []Address{{Email: value}}
	}
	out := make([]Address, 0, len(parsed))
	for _, address := range parsed {
		out = append(out, Address{
			Name:  strings.TrimSpace(address.Name),
			Email: strings.TrimSpace(address.Address),
		})
	}
	return out
}

func internalDate(value string) time.Time {
	ms, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func attachments(part messagePart) []Attachment {
	var out []Attachment
	var walk func(messagePart)
	walk = func(part messagePart) {
		if part.Filename != "" || part.Body.AttachmentID != "" {
			inline := inlinePart(part)
			if !(inline && strings.HasPrefix(strings.ToLower(part.MIMEType), "image/")) {
				out = append(out, Attachment{
					ID:       part.Body.AttachmentID,
					FileName: part.Filename,
					MIMEType: part.MIMEType,
					Size:     part.Body.Size,
					Inline:   inline,
				})
			}
		}
		for _, child := range part.Parts {
			walk(child)
		}
	}
	walk(part)
	return out
}

func inlinePart(part messagePart) bool {
	if part.Filename == "" {
		return true
	}
	headers := headersByName(part.Headers)
	disposition := strings.ToLower(headers["content-disposition"])
	if strings.HasPrefix(disposition, "inline") {
		return true
	}
	return headers["content-id"] != "" && strings.HasPrefix(strings.ToLower(part.MIMEType), "image/")
}

func messageBodies(part messagePart) (string, string) {
	var textParts []string
	var htmlParts []string
	var walk func(messagePart)
	walk = func(part messagePart) {
		switch strings.ToLower(strings.TrimSpace(part.MIMEType)) {
		case "text/plain":
			if text := decodeBody(part.Body.Data); text != "" {
				textParts = append(textParts, text)
			}
		case "text/html":
			if html := decodeBody(part.Body.Data); html != "" {
				htmlParts = append(htmlParts, html)
			}
		}
		for _, child := range part.Parts {
			walk(child)
		}
	}
	walk(part)
	return strings.Join(textParts, "\n\n"), strings.Join(htmlParts, "\n\n")
}

func decodeBody(data string) string {
	raw, err := decodeBodyBytes(data)
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeBodyBytes(data string) ([]byte, error) {
	if data == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(data)
	}
	if err != nil {
		return nil, err
	}
	return raw, nil
}
