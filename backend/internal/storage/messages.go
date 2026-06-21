package storage

import (
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
)

func TextBlock(text string) Block {
	return Block{Type: BlockTypeText, Text: text}
}

func QuoteBlock(text string) Block {
	return Block{Type: BlockTypeQuote, Text: text}
}

func AttachmentBlock(attachment Attachment) Block {
	block := Block{
		Type:       BlockTypeAttachment,
		ID:         attachment.ID,
		Name:       attachment.Name,
		MimeType:   attachment.MimeType,
		Size:       attachment.Size,
		ServerPath: attachment.ServerPath,
	}
	if block.ServerPath == "" {
		block.URI = attachment.URI
	}
	return block
}

func UserMessageRecord(message string, quotes []string, attachments []Attachment) Message {
	blocks := make([]Block, 0, len(quotes)+1+len(attachments))
	for _, quote := range quotes {
		blocks = append(blocks, QuoteBlock(quote))
	}
	blocks = append(blocks, TextBlock(message))
	for _, attachment := range attachments {
		blocks = append(blocks, AttachmentBlock(attachment))
	}
	return Message{
		Role:    "user",
		Content: message,
		Blocks:  blocks,
	}
}

func AppendUserMessage(store MessageAppender, sessionID, message string, quotes []string, attachments []Attachment) error {
	if len(attachments) > 0 || len(quotes) > 0 {
		if appender, ok := store.(MessageRecordAppender); ok {
			return appender.AppendMessageRecords(sessionID, UserMessageRecord(message, quotes, attachments))
		}
	}
	return store.AppendMessages(sessionID, provider.UserMessage(message))
}

func MergeDurableBlocks(record, existing Message) Message {
	if record.Role != existing.Role || record.Content != existing.Content {
		return record
	}
	record = mergeToolMediaRefs(record, existing)
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

func MediaRefsByToolCall(records []Message) map[string][]media.Ref {
	out := map[string][]media.Ref{}
	for _, record := range records {
		for _, block := range record.Blocks {
			if block.Type != BlockTypeTool || block.ID == "" || len(block.MediaRefs) == 0 {
				continue
			}
			out[block.ID] = media.CloneRefs(block.MediaRefs)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeToolMediaRefs(record, existing Message) Message {
	refsByID := map[string][]media.Ref{}
	for _, block := range existing.Blocks {
		if block.Type == BlockTypeTool && block.ID != "" && len(block.MediaRefs) > 0 {
			refsByID[block.ID] = media.CloneRefs(block.MediaRefs)
		}
	}
	if len(refsByID) == 0 {
		return record
	}
	for i := range record.Blocks {
		block := &record.Blocks[i]
		if block.Type != BlockTypeTool || block.ID == "" || len(block.MediaRefs) > 0 {
			continue
		}
		if refs := refsByID[block.ID]; len(refs) > 0 {
			block.MediaRefs = media.CloneRefs(refs)
		}
	}
	return record
}

func DurableBlock(block Block) bool {
	return block.Type == BlockTypeAttachment
}

func durableBlockKey(block Block) string {
	if block.ID != "" {
		return block.Type + ":" + block.ID
	}
	if block.ServerPath != "" {
		return block.Type + ":" + block.Name + ":" + block.ServerPath
	}
	return block.Type + ":" + block.Name + ":" + block.URI
}
