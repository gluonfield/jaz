package storage

import (
	"encoding/json"
	"fmt"
	"strings"
)

type QueuedMessage struct {
	Text          string   `json:"text"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
	PlanRequested bool     `json:"plan_requested,omitempty"`
}

func NormalizeQueuedMessages(messages []QueuedMessage) []QueuedMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]QueuedMessage, 0, len(messages))
	for _, message := range messages {
		message.Text = strings.TrimSpace(message.Text)
		message.AttachmentIDs = normalizeQueuedAttachmentIDs(message.AttachmentIDs)
		if message.Text == "" {
			continue
		}
		out = append(out, message)
	}
	return out
}

func NewQueuedMessage(text string, attachmentIDs []string) QueuedMessage {
	return QueuedMessage{Text: strings.TrimSpace(text), AttachmentIDs: normalizeQueuedAttachmentIDs(attachmentIDs)}
}

func normalizeQueuedAttachmentIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func MarshalQueuedMessages(messages []QueuedMessage) (string, error) {
	messages = NormalizeQueuedMessages(messages)
	if len(messages) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UnmarshalQueuedMessages(raw string) ([]QueuedMessage, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var messages []QueuedMessage
	if err := json.Unmarshal([]byte(raw), &messages); err != nil {
		return nil, fmt.Errorf("queued messages: %w", err)
	}
	return NormalizeQueuedMessages(messages), nil
}
