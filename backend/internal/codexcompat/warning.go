package codexcompat

import (
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

var hiddenWarningPrefixes = []string{
	"Falling back from WebSockets to HTTPS transport.",
	"Model metadata for `",
}

func IsHiddenWarning(messageID, message string) bool {
	messageID = strings.TrimPrefix(messageID, "message:")
	if !strings.HasPrefix(messageID, "codex:warning:") {
		return false
	}
	for _, prefix := range hiddenWarningPrefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func IsHiddenWarningEvent(event sessionevents.Event) bool {
	return event.Type == sessionevents.TypeACPMessage && event.ACP != nil &&
		IsHiddenWarning(event.ACP.TextRunID, event.Content)
}
