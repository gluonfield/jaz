package server

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type sessionActions struct {
	Compact bool `json:"compact,omitempty"`
}

func knownSessionAction(action string) bool {
	switch action {
	case "messages:stream",
		"attachments",
		"archive",
		"unarchive",
		"pin",
		"unpin",
		"rename",
		"interactive-response",
		"permission",
		"cancel",
		"compact",
		"queue",
		"repo/push",
		"repo/commit",
		"repo/merge",
		"repo/merge-from-main",
		"repo/restore-worktree":
		return true
	default:
		return false
	}
}

func sessionActionsForSession(session storage.Session) sessionActions {
	if !sessionSupportsCompact(session) {
		return sessionActions{}
	}
	return sessionActions{Compact: true}
}

func sessionSupportsCompact(session storage.Session) bool {
	if session.Runtime != storage.RuntimeACP {
		return false
	}
	agent := ""
	if session.RuntimeRef != nil {
		agent = session.RuntimeRef.Agent
	}
	if agent == "" {
		agent = session.ModelProvider
	}
	return acp.AgentSupportsCompact(agent)
}
