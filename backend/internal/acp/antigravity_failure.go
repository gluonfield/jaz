package acp

import (
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

// Antigravity has no failure record: once its own retries run out it sends
// the reason as a last agent message, "\n⚠ <reason>\n", and ends the prompt
// with a plain end_turn.
const antigravityFailurePrefix = "\n⚠ "

// antigravityRetryable are the prefixes of the reasons a later attempt can get
// past: the connection to the model dropping, and Vertex AI's shared capacity
// throttling the request.
var antigravityRetryable = []string{
	"There was a network issue connecting to the server",
	"Resource exhausted.",
}

// antigravityFailure is Antigravity's failure message read as the failure an
// AIR agent puts on its prompt response.
func antigravityFailure(agent string, update acpschema.DecodedSessionUpdate) *sessionFailure {
	message, ok := update.(acpschema.AgentMessageChunkUpdate)
	if !ok || CanonicalAgentName(agent) != AgentAntigravity {
		return nil
	}
	reason, prefixed := strings.CutPrefix(contentText(message.Content), antigravityFailurePrefix)
	reason, line := strings.CutSuffix(reason, "\n")
	if !prefixed || !line {
		return nil
	}
	failure := &sessionFailure{Title: reason}
	for _, retryable := range antigravityRetryable {
		if strings.HasPrefix(reason, retryable) {
			failure.Actions = []string{"retry"}
		}
	}
	return failure
}

// holdAgentFailure keeps a failure the agent sent as a message for the end of
// its prompt call, instead of showing it as the answer, and reports whether
// it did.
func (j *jobState) holdAgentFailure(update acpschema.DecodedSessionUpdate) bool {
	failure := antigravityFailure(j.ACPAgent, update)
	if failure == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn == nil {
		return false
	}
	j.turn.agentFailure = failure
	return true
}
