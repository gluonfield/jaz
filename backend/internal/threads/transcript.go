package threads

import (
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type TranscriptMessage struct {
	Role      string           `json:"role"`
	Text      string           `json:"text,omitempty"`
	Tools     []TranscriptTool `json:"tools,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

type TranscriptTool struct {
	Name   string `json:"name,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func TranscriptFromRecords(records []storage.Message, maxToolChars int) []TranscriptMessage {
	messages := make([]TranscriptMessage, 0, len(records))
	for _, record := range records {
		if record.Role != "user" && record.Role != "assistant" {
			continue
		}
		message := TranscriptMessage{
			Role:      record.Role,
			Text:      transcriptText(record),
			CreatedAt: record.CreatedAt,
		}
		if maxToolChars > 0 {
			message.Tools = compressedTranscriptTools(record.Blocks, maxToolChars)
		}
		if message.Text == "" && len(message.Tools) == 0 {
			continue
		}
		messages = append(messages, message)
	}
	return messages
}

func ToolCounts(records []storage.Message) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		for _, block := range record.Blocks {
			if block.Type != storage.BlockTypeTool {
				continue
			}
			name := block.Name
			if name == "" {
				name = "unknown"
			}
			counts[name]++
		}
	}
	return counts
}

func transcriptText(record storage.Message) string {
	if text := strings.TrimSpace(record.Content); text != "" {
		return text
	}
	var parts []string
	for _, block := range record.Blocks {
		if block.Type != storage.BlockTypeText {
			continue
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func compressedTranscriptTools(blocks []storage.Block, maxChars int) []TranscriptTool {
	var tools []TranscriptTool
	for _, block := range blocks {
		if block.Type != storage.BlockTypeTool {
			continue
		}
		detail := compressedToolDetail(block)
		if len(detail) > maxChars {
			detail = detail[:maxChars] + "..."
		}
		tools = append(tools, TranscriptTool{Name: block.Name, Detail: detail})
	}
	return tools
}

func compressedToolDetail(block storage.Block) string {
	detail := strings.TrimSpace(block.InputJSON)
	if result := strings.TrimSpace(block.Result); result != "" {
		if detail != "" {
			detail += " -> "
		}
		detail += result
	}
	return strings.Join(strings.Fields(detail), " ")
}
