package threads

import (
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func mergeContextEvents(id string, records []contextRecord, events []sessionevents.Event, opts contextOptions, counts map[string]int) []contextRecord {
	var outputs []contextRecord
	for _, event := range sessionevents.CompactTranscript(storage.DisplayEvents(events)) {
		message := ContextMessage{Role: "assistant", CreatedAt: event.At}
		searchText := ""
		switch {
		case event.Type == sessionevents.TypeRoomMessage:
			if room := event.RoomMessage; event.SessionID == id && room != nil && room.Speaker == "bot" {
				message.Text = room.Text
			}
		case event.ACP == nil || event.ACP.ID != id:
			continue
		case event.Type == sessionevents.TypeACPMessage:
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
			outputs = append(outputs, contextRecord{message: message, searchText: strings.ToLower(searchText)})
		}
	}
	if len(outputs) == 0 {
		return records
	}
	merged := make([]contextRecord, 0, len(records)+len(outputs))
	for len(records) > 0 && len(outputs) > 0 {
		if outputs[0].message.CreatedAt.Before(records[0].message.CreatedAt) {
			merged = append(merged, outputs[0])
			outputs = outputs[1:]
		} else {
			merged = append(merged, records[0])
			records = records[1:]
		}
	}
	merged = append(merged, records...)
	merged = append(merged, outputs...)
	for i := range merged {
		merged[i].message.Seq = int64(i + 1)
	}
	return merged
}
