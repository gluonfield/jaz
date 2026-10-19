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

// providerRetry is a failure, by incident and category, that the provider
// retried itself. Agents keep one record per incident, so a transport retry
// and the capacity failure after it share an id; the category tells them
// apart.
type providerRetry struct {
	id, category string
}

// providerRetryDelays are the waits before each continuation Jaz sends after
// a provider failure the provider did not retry itself.
var providerRetryDelays = []time.Duration{15 * time.Second, time.Minute, 4 * time.Minute}

func sessionFailureRecord(raw json.RawMessage) *sessionFailure {
	var update struct {
		Meta sessionFailureMeta `json:"_meta"`
	}
	if json.Unmarshal(raw, &update) != nil {
		return nil
	}
	return update.Meta.Jetbrains.Air.SessionFailure
}

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

// noticeAsMessage is a notice record, such as a configuration warning or a
// model fallback, as the message chunk agents write for clients without the
// extension; the provider's own retries are warnings of a failure category.
func noticeAsMessage(update acpschema.DecodedSessionUpdate, raw json.RawMessage) acpschema.DecodedSessionUpdate {
	failure := sessionFailureRecord(raw)
	if _, info := update.(acpschema.SessionInfoSessionUpdate); !info || failure == nil || failure.Severity != "warning" || failure.Category != "unknown" {
		return update
	}
	block, err := marshalContentBlock(acpschema.TextContentBlock{
		Kind:        acpschema.ContentBlockText,
		TextContent: acpschema.TextContent{Text: "Warning: " + failure.message() + "\n\n"},
	})
	if err != nil {
		return update
	}
	return acpschema.AgentMessageChunkUpdate{ContentChunk: acpschema.ContentChunk{Content: block}}
}

// retryDelay is how long to wait before continuing a turn this failure ended,
// when the agent offers a retry, the provider did not already retry it, and
// the turn has tries left.
func (t *activeTurn) retryDelay(failure sessionFailure) (time.Duration, bool) {
	if !slices.Contains(failure.Actions, "retry") || t.providerRetried[providerRetry{failure.ID, failure.Category}] || t.retries >= len(providerRetryDelays) {
		return 0, false
	}
	return providerRetryDelays[t.retries], true
}

func (t *activeTurn) markProviderRetried(failure sessionFailure) {
	if t.providerRetried == nil {
		t.providerRetried = map[providerRetry]bool{}
	}
	t.providerRetried[providerRetry{failure.ID, failure.Category}] = true
}

func retryNotice(failure sessionFailure, delay time.Duration, attempt int) string {
	wait := fmt.Sprintf("%d s", int(delay.Seconds()))
	if delay >= time.Minute {
		wait = fmt.Sprintf("%d min", int(delay.Minutes()))
	}
	return fmt.Sprintf("%s Retrying in %s (%d of %d).", failure.Title, wait, attempt, len(providerRetryDelays))
}
