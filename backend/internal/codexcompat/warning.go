package codexcompat

import (
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

// Codex reports warnings as ordinary agent message chunks. The app-server
// adapter prefixes them with "Warning: " and sends no message id; the older
// adapter sent the bare text under a "codex:warning:" id. Match the text alone
// so both shapes are covered.
const warningPrefix = "Warning: "

var hiddenWarningPrefixes = []string{
	"Falling back from WebSockets to HTTPS transport.",
}

func IsHiddenWarning(message string) bool {
	message = strings.TrimPrefix(message, warningPrefix)
	for _, prefix := range hiddenWarningPrefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func IsHiddenWarningEvent(event sessionevents.Event) bool {
	return event.Type == sessionevents.TypeACPMessage && event.ACP != nil && IsHiddenWarning(event.Content)
}
