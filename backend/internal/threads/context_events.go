package threads

import (
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func mergeContextEvents(id string, records []contextRecord, events []sessionevents.Event, opts contextOptions, counts map[string]int, botChat bool) []contextRecord {
	recordCount := len(records)
	for _, event := range sessionevents.CompactTranscript(storage.DisplayEvents(events)) {
		message := ContextMessage{Role: "assistant", CreatedAt: event.At}
		searchText := ""
		switch {
		case botChat && event.SessionID == id && event.RoomMessage != nil && event.RoomMessage.Speaker == "bot":
			message.Text = event.RoomMessage.Text
		case event.ACP == nil || event.ACP.ID != id:
			continue
		case !botChat && event.Type == sessionevents.TypeACPMessage:
			message.Text = event.Content
		case event.Type == "acp_tool":
			for _, call := range event.ACP.ToolCalls {
				name := firstNonEmpty(call.ToolName, call.Title, "unknown")
				counts[name]++
				searchText += " " + name
				message.Tools = append(message.Tools, ContextTool{Name: name})
			}
		}
		searchText += message.Text
		message.Text, message.Truncated = clampText(message.Text, opts.maxTextChars)
		if message.Text != "" || len(message.Tools) > 0 {
			records = append(records, contextRecord{message: message, searchText: strings.ToLower(searchText)})
		}
	}
	if len(records) == recordCount {
		return records
	}
	slices.SortStableFunc(records, func(a, b contextRecord) int {
		return a.message.CreatedAt.Compare(b.message.CreatedAt)
	})
	for i := range records {
		records[i].message.Seq = int64(i + 1)
	}
	return records
}
