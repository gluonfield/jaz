package server

import (
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

func nativeMessageWithAttachmentLinks(message string, attachments []storage.Attachment) string {
	if len(attachments) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	b.WriteString("\n\nAttachments:\n")
	for _, attachment := range attachments {
		fmt.Fprintf(&b, "- %s: %s\n", attachment.Name, attachment.URI)
	}
	return strings.TrimRight(b.String(), "\n")
}
