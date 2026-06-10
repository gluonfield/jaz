package storage

import "github.com/wins/jaz/backend/internal/provider"

func TextBlock(text string) Block {
	return Block{Type: BlockTypeText, Text: text}
}

func AttachmentBlock(attachment Attachment) Block {
	return Block{
		Type:       BlockTypeAttachment,
		ID:         attachment.ID,
		Name:       attachment.Name,
		URI:        attachment.URI,
		MimeType:   attachment.MimeType,
		Size:       attachment.Size,
		ServerPath: attachment.ServerPath,
	}
}

func UserMessageRecord(message string, attachments []Attachment) Message {
	blocks := []Block{TextBlock(message)}
	for _, attachment := range attachments {
		blocks = append(blocks, AttachmentBlock(attachment))
	}
	return Message{
		Role:    "user",
		Content: message,
		Blocks:  blocks,
	}
}

func AppendUserMessage(store MessageAppender, sessionID, message string, attachments []Attachment) error {
	if len(attachments) > 0 {
		if appender, ok := store.(MessageRecordAppender); ok {
			return appender.AppendMessageRecords(sessionID, UserMessageRecord(message, attachments))
		}
	}
	return store.AppendMessages(sessionID, provider.UserMessage(message))
}

func MergeDurableBlocks(record, existing Message) Message {
	if record.Role != existing.Role || record.Content != existing.Content {
		return record
	}
	var missing []Block
	seen := map[string]bool{}
	for _, block := range record.Blocks {
		if !DurableBlock(block) {
			continue
		}
		seen[durableBlockKey(block)] = true
	}
	for _, block := range existing.Blocks {
		if !DurableBlock(block) {
			continue
		}
		key := durableBlockKey(block)
		if seen[key] {
			continue
		}
		missing = append(missing, block)
		seen[key] = true
	}
	if len(missing) == 0 {
		return record
	}
	record.Blocks = append(record.Blocks, missing...)
	return record
}

func DurableBlock(block Block) bool {
	return block.Type == BlockTypeAttachment
}

func durableBlockKey(block Block) string {
	if block.ID != "" {
		return block.Type + ":" + block.ID
	}
	return block.Type + ":" + block.Name + ":" + block.URI
}
