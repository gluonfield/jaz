package acp

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

// sessionFailure is a provider failure an agent reports through the AIR
// sessionFailure extension Jaz negotiates at initialize. A turn's terminal
// failure rides on its prompt response; records the agent sends while a turn
// runs are the provider retrying on its own, or notices.
type sessionFailure struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Details  string   `json:"details"`
	Actions  []string `json:"actions"`
}

type sessionFailureMeta struct {
	Jetbrains struct {
		Air struct {
			SessionFailure *sessionFailure `json:"sessionFailure"`
		} `json:"air"`
	} `json:"jetbrains"`
}

// providerRetryDelays are the waits before each continuation Jaz sends after
// a provider failure the provider did not retry itself.
var providerRetryDelays = []time.Duration{15 * time.Second, time.Minute, 4 * time.Minute}

// terminal is the failure that ended a turn; AIR reads an absent severity as
// an error.
func (m sessionFailureMeta) terminal() *sessionFailure {
	failure := m.Jetbrains.Air.SessionFailure
	if failure == nil || failure.Severity == "warning" {
		return nil
	}
	return failure
}

func (f sessionFailure) message() string {
	if f.Details == "" {
		return f.Title
	}
	return f.Title + "\n\n" + f.Details
}

// sessionFailureUpdate applies a failure record a session_info_update
// carries. A warning of a failure category is the provider retrying that
// failure itself; a warning without one is a notice, such as a configuration
// warning, a deprecation or a model fallback, which becomes the "Warning:"
// message agents write for clients without the extension.
func sessionFailureUpdate(job *jobState, update acpschema.DecodedSessionUpdate, raw json.RawMessage) acpschema.DecodedSessionUpdate {
	if _, ok := update.(acpschema.SessionInfoSessionUpdate); !ok {
		return update
	}
	var info struct {
		Meta sessionFailureMeta `json:"_meta"`
	}
	if json.Unmarshal(raw, &info) != nil {
		return update
	}
	failure := info.Meta.Jetbrains.Air.SessionFailure
	if failure == nil || failure.Severity != "warning" {
		return update
	}
	if failure.Category != "unknown" {
		job.mu.Lock()
		if job.turn != nil {
			job.turn.markProviderRetried(failure.Category)
		}
		job.mu.Unlock()
		return update
	}
	// A struct of strings always marshals.
	block, _ := marshalContentBlock(acpschema.TextContentBlock{
		Kind:        acpschema.ContentBlockText,
		TextContent: acpschema.TextContent{Text: "Warning: " + failure.message() + "\n\n"},
	})
	return acpschema.AgentMessageChunkUpdate{ContentChunk: acpschema.ContentChunk{Content: block}}
}

// retryDelay is how long to wait before continuing a turn this failure ended,
// when the agent offers a retry, the provider did not already retry it, and
// the turn has tries left. The provider retried it when, since the turn last
// made progress, it reported retrying a failure of the same category; agents
// differ in how they identify one failure across its records.
func (t *activeTurn) retryDelay(failure sessionFailure) (time.Duration, bool) {
	if !slices.Contains(failure.Actions, "retry") || t.providerRetried[failure.Category] || t.retries >= len(providerRetryDelays) {
		return 0, false
	}
	return providerRetryDelays[t.retries], true
}

func (t *activeTurn) markProviderRetried(category string) {
	if t.providerRetried == nil {
		t.providerRetried = map[string]bool{}
	}
	t.providerRetried[category] = true
}

func retryNotice(failure sessionFailure, delay time.Duration, attempt int) string {
	wait := fmt.Sprintf("%d s", int(delay.Seconds()))
	if delay >= time.Minute {
		wait = fmt.Sprintf("%d min", int(delay.Minutes()))
	}
	return fmt.Sprintf("%s Retrying in %s (%d of %d).", failure.Title, wait, attempt, len(providerRetryDelays))
}
