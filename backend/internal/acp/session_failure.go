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

// sessionWarning is the warning record a session_info_update of the turn
// carries: the provider retrying a failure of its category itself, or a
// notice. A side chat's records are its own, not the turn's.
func sessionWarning(jobID string, update acpschema.DecodedSessionUpdate, raw json.RawMessage) *sessionFailure {
	info, ok := update.(acpschema.SessionInfoSessionUpdate)
	if !ok {
		return nil
	}
	if _, side := sideChatScopeFromMeta(info.Meta, jobID); side {
		return nil
	}
	var record struct {
		Meta sessionFailureMeta `json:"_meta"`
	}
	if json.Unmarshal(raw, &record) != nil {
		return nil
	}
	if failure := record.Meta.Jetbrains.Air.SessionFailure; failure != nil && failure.Severity == "warning" {
		return failure
	}
	return nil
}

// notice is a warning without a failure category, such as a configuration
// warning, a deprecation or a model fallback.
func (f sessionFailure) notice() bool {
	return f.Category == "unknown"
}

// noticeMessage is a notice as the "Warning:" message agents write for
// clients without the extension.
func noticeMessage(notice sessionFailure) acpschema.DecodedSessionUpdate {
	// A struct of strings always marshals.
	block, _ := marshalContentBlock(acpschema.TextContentBlock{
		Kind:        acpschema.ContentBlockText,
		TextContent: acpschema.TextContent{Text: "Warning: " + notice.message() + "\n\n"},
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

// trackProviderRetries records a failure the provider reports retrying, and
// forgets them once the turn produces output, which ends the provider's
// retries.
func (t *activeTurn) trackProviderRetries(warning *sessionFailure, output bool) {
	switch {
	case warning != nil && !warning.notice():
		if t.providerRetried == nil {
			t.providerRetried = map[string]bool{}
		}
		t.providerRetried[warning.Category] = true
	case output:
		t.providerRetried = nil
	}
}

func retryNotice(failure sessionFailure, delay time.Duration, attempt int) string {
	wait := fmt.Sprintf("%d s", int(delay.Seconds()))
	if delay >= time.Minute {
		wait = fmt.Sprintf("%d min", int(delay.Minutes()))
	}
	return fmt.Sprintf("%s Retrying in %s (%d of %d).", failure.Title, wait, attempt, len(providerRetryDelays))
}
