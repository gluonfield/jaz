package acp

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/wins/jaz/backend/internal/codexcompat"
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

func sessionFailureRecord(raw json.RawMessage) *sessionFailure {
	var update struct {
		Meta sessionFailureMeta `json:"_meta"`
	}
	if json.Unmarshal(raw, &update) != nil {
		return nil
	}
	return update.Meta.Jetbrains.Air.SessionFailure
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

// notice is a record that only informs, such as a configuration warning or a
// model fallback; the provider's own retries are warnings of a failure kind.
func (f sessionFailure) notice() bool {
	return f.Severity == "warning" && f.Category == "unknown"
}

// noticeText is how agents write a notice into the transcript for clients
// without the extension.
func (f sessionFailure) noticeText() string {
	text := "Warning: " + f.Title
	if f.Details != "" {
		text += "\n\n" + f.Details
	}
	if codexcompat.IsHiddenWarning(text) {
		return ""
	}
	return text + "\n\n"
}

func (f sessionFailure) message() string {
	if f.Details == "" {
		return f.Title
	}
	return f.Title + "\n\n" + f.Details
}

// markProviderRetried notes a failure the provider is retrying itself, so a
// turn the same failure ends is not retried again. Agents keep one record per
// incident, so a transport retry and the capacity failure after it share an
// id; the category tells them apart.
func (t *activeTurn) markProviderRetried(failure sessionFailure) {
	if t.providerRetried == nil {
		t.providerRetried = map[string]string{}
	}
	t.providerRetried[failure.ID] = failure.Category
}

// retryDelay is how long to wait before continuing a turn this failure ended,
// when the agent offers a retry, the provider did not already retry it, and
// the turn has tries left.
func (t *activeTurn) retryDelay(failure sessionFailure) (time.Duration, bool) {
	category, retried := t.providerRetried[failure.ID]
	if !slices.Contains(failure.Actions, "retry") || retried && category == failure.Category || t.retries >= len(providerRetryDelays) {
		return 0, false
	}
	return providerRetryDelays[t.retries], true
}

func retryNotice(failure sessionFailure, delay time.Duration, attempt int) string {
	wait := fmt.Sprintf("%d s", int(delay.Seconds()))
	if delay >= time.Minute {
		wait = fmt.Sprintf("%d min", int(delay.Minutes()))
	}
	return fmt.Sprintf("%s Retrying in %s (%d of %d).", failure.Title, wait, attempt, len(providerRetryDelays))
}
