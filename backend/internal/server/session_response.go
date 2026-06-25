package server

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func canonicalSessionResponses(sessions []storage.Session) []storage.Session {
	out := make([]storage.Session, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, canonicalSessionResponse(session))
	}
	return out
}

func canonicalSessionResponse(session storage.Session) storage.Session {
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return session
	}
	ref := *session.RuntimeRef
	canonical := acp.CanonicalAgentName(ref.Agent)
	if canonical == "" {
		session.RuntimeRef = &ref
		return session
	}
	if session.ModelProvider != "" && acp.CanonicalAgentName(session.ModelProvider) == canonical {
		session.ModelProvider = canonical
	}
	ref.Agent = canonical
	ref.Capabilities = storage.NormalizeRuntimeCapabilities(ref.Capabilities)
	if ref.Capabilities == nil && nativeGoalSupport(session) == promptFeatureNegotiable {
		ref.Capabilities = &storage.RuntimeCapabilities{NativeGoalNegotiable: true}
	}
	session.RuntimeRef = &ref
	return session
}
