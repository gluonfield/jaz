package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	blockText       = storage.BlockTypeText
	blockReasoning  = storage.BlockTypeReasoning
	blockTool       = storage.BlockTypeTool
	blockAttachment = storage.BlockTypeAttachment
)

func recordsFromProviderMessages(messages []provider.Message, start time.Time) ([]storage.Message, error) {
	return recordsFromProviderMessagesWithReasoning(messages, nil, start)
}

func recordsFromProviderMessagesWithReasoning(messages []provider.Message, reasoningByMessage map[int]string, start time.Time) ([]storage.Message, error) {
	return recordsFromProviderMessagesWithReasoningAndMedia(messages, reasoningByMessage, nil, start)
}

func recordsFromProviderMessagesWithReasoningAndMedia(messages []provider.Message, reasoningByMessage map[int]string, mediaRefs map[string][]media.Ref, start time.Time) ([]storage.Message, error) {
	records := make([]storage.Message, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		msgIndex := i
		role := provider.MessageRole(msg)
		switch role {
		case "assistant":
			calls := provider.MessageToolCalls(msg)
			if len(calls) == 0 {
				records = append(records, messageRecord(role, provider.MessageContent(msg), textBlocks(provider.MessageContent(msg), reasoningByMessage[msgIndex]), start, len(records)+1))
				continue
			}
			blocks := textBlocks(provider.MessageContent(msg), reasoningByMessage[msgIndex])
			for _, call := range calls {
				// Mid-turn snapshots store calls without results; the next save fills them in.
				if i+1 >= len(messages) || provider.MessageRole(messages[i+1]) != "tool" {
					blocks = append(blocks, storage.Block{
						Type:      blockTool,
						ID:        provider.ToolCallID(call),
						Name:      provider.ToolCallName(call),
						InputJSON: provider.ToolCallArguments(call),
						MediaRefs: media.CloneRefs(mediaRefs[provider.ToolCallID(call)]),
					})
					continue
				}
				i++
				result := messages[i]
				resultID := provider.MessageToolCallID(result)
				if resultID != provider.ToolCallID(call) {
					return nil, fmt.Errorf("assistant tool call %q followed by result for %q", provider.ToolCallID(call), resultID)
				}
				blocks = append(blocks, storage.Block{
					Type:      blockTool,
					ID:        provider.ToolCallID(call),
					Name:      provider.ToolCallName(call),
					InputJSON: provider.ToolCallArguments(call),
					Result:    provider.MessageContent(result),
					MediaRefs: media.CloneRefs(mediaRefs[provider.ToolCallID(call)]),
				})
			}
			records = append(records, messageRecord(role, provider.MessageContent(msg), blocks, start, len(records)+1))
		case "tool":
			return nil, fmt.Errorf("tool result %q has no preceding assistant tool call", provider.MessageToolCallID(msg))
		case "system", "developer", "user":
			records = append(records, messageRecord(role, provider.MessageContent(msg), nil, start, len(records)+1))
		case "":
			return nil, fmt.Errorf("message has no role")
		default:
			records = append(records, messageRecord(role, provider.MessageContent(msg), nil, start, len(records)+1))
		}
	}
	return records, nil
}

func providerMessagesFromRecords(records []storage.Message) ([]provider.Message, error) {
	messages := make([]provider.Message, 0, len(records))
	for _, record := range records {
		blocks := record.Blocks
		if len(blocks) == 0 && strings.TrimSpace(record.Content) != "" {
			blocks = textBlocks(record.Content, record.Reasoning)
		}
		content := blocksText(blocks)
		if content == "" {
			content = record.Content
		}
		switch record.Role {
		case "system":
			messages = append(messages, provider.SystemMessage(content))
		case "developer":
			messages = append(messages, provider.DeveloperMessage(content))
		case "user":
			messages = append(messages, provider.UserMessage(content))
		case "assistant":
			var calls []provider.ToolCall
			var results []storage.Block
			for _, block := range blocks {
				if block.Type != blockTool {
					continue
				}
				calls = append(calls, provider.FunctionToolCall(block.ID, block.Name, block.InputJSON))
				results = append(results, block)
			}
			messages = append(messages, provider.AssistantMessage(content, calls))
			for _, result := range results {
				// An empty result means the turn was interrupted mid-tool.
				text := result.Result
				if text == "" {
					text = `{"status":"interrupted","error":"no result was recorded"}`
				}
				messages = append(messages, provider.ToolMessage(text, result.ID))
			}
		case "tool":
			return nil, fmt.Errorf("canonical message row %d must not use role tool", record.Seq)
		default:
			if content != "" {
				messages = append(messages, provider.UserMessage(content))
			}
		}
	}
	return messages, nil
}

func messageRecord(role, content string, blocks []storage.Block, start time.Time, seq int) storage.Message {
	if blocks == nil {
		blocks = textBlocks(content, "")
	}
	return storage.Message{
		Seq:       int64(seq),
		Role:      role,
		Content:   blocksText(blocks),
		Reasoning: blocksReasoning(blocks),
		Blocks:    blocks,
		CreatedAt: start.Add(time.Duration(seq) * time.Millisecond),
	}
}

func textBlocks(content, reasoning string) []storage.Block {
	blocks := make([]storage.Block, 0, 2)
	if reasoning != "" {
		blocks = append(blocks, storage.Block{Type: blockReasoning, Text: reasoning})
	}
	if content != "" {
		blocks = append(blocks, storage.Block{Type: blockText, Text: content})
	}
	return blocks
}

func marshalBlocks(blocks []storage.Block) (string, error) {
	if len(blocks) == 0 {
		return "[]", nil
	}
	if err := validateBlocks(blocks); err != nil {
		return "", err
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalBlocks(raw string) ([]storage.Block, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var blocks []storage.Block
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return nil, err
	}
	return blocks, validateBlocks(blocks)
}

func validateBlocks(blocks []storage.Block) error {
	for i, block := range blocks {
		switch block.Type {
		case blockText, blockReasoning:
		case blockAttachment:
			if block.ID == "" {
				return fmt.Errorf("attachment block %d missing id", i)
			}
			if block.Name == "" {
				return fmt.Errorf("attachment block %d missing name", i)
			}
			if block.URI == "" {
				return fmt.Errorf("attachment block %d missing uri", i)
			}
		case blockTool:
			if block.ID == "" {
				return fmt.Errorf("tool block %d missing id", i)
			}
			if block.Name == "" {
				return fmt.Errorf("tool block %d missing name", i)
			}
		default:
			return fmt.Errorf("unknown block type %q", block.Type)
		}
	}
	return nil
}

func blocksText(blocks []storage.Block) string {
	var out []string
	for _, block := range blocks {
		if block.Type == blockText && block.Text != "" {
			out = append(out, block.Text)
		}
	}
	return strings.Join(out, "")
}

func blocksReasoning(blocks []storage.Block) string {
	var out []string
	for _, block := range blocks {
		if block.Type == blockReasoning && block.Text != "" {
			out = append(out, block.Text)
		}
	}
	return strings.Join(out, "\n\n")
}
