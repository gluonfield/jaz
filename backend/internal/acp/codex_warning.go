package acp

import (
	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/codexcompat"
)

func codexHiddenWarning(agent string, update acpschema.DecodedSessionUpdate) bool {
	message, ok := update.(acpschema.AgentMessageChunkUpdate)
	return ok && CanonicalAgentName(agent) == AgentCodex &&
		codexcompat.IsHiddenWarning(message.MessageID, contentText(message.Content))
}
