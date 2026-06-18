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
		normalized, ok := NormalizeQueuedMessage(message)
		if !ok {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func NormalizeQueuedMessage(message QueuedMessage) (QueuedMessage, bool) {
	message.Text = strings.TrimSpace(message.Text)
	message.AttachmentIDs = normalizeQueuedAttachmentIDs(message.AttachmentIDs)
	return message, message.Text != ""
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

func MarshalQueuedMessage(message *QueuedMessage) (string, error) {
	if message == nil {
		return "", nil
	}
	normalized, ok := NormalizeQueuedMessage(*message)
	if !ok {
		return "", nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UnmarshalQueuedMessages(raw string) ([]QueuedMessage, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("queued messages: %w", err)
	}
	messages := make([]QueuedMessage, 0, len(entries))
	for _, entry := range entries {
		var message QueuedMessage
		if err := json.Unmarshal(entry, &message); err == nil {
			messages = append(messages, message)
			continue
		}
		var text string
		if err := json.Unmarshal(entry, &text); err != nil {
			return nil, fmt.Errorf("queued messages: %w", err)
		}
		messages = append(messages, NewQueuedMessage(text, nil))
	}
	return NormalizeQueuedMessages(messages), nil
}

func UnmarshalQueuedMessage(raw string) (*QueuedMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var message QueuedMessage
	if err := json.Unmarshal([]byte(raw), &message); err == nil {
		normalized, ok := NormalizeQueuedMessage(message)
		if !ok {
			return nil, nil
		}
		return &normalized, nil
	}
	var text string
	if err := json.Unmarshal([]byte(raw), &text); err != nil {
		return nil, fmt.Errorf("queued message: %w", err)
	}
	normalized, ok := NormalizeQueuedMessage(NewQueuedMessage(text, nil))
	if !ok {
		return nil, nil
	}
	return &normalized, nil
}
