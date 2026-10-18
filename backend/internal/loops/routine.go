package loops

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// runPrompt is what a run sends: why it runs, then the routine's prompt, then
// how a bot's routine delivers. A turn in the bot's own thread is also labelled
// with the routine and the local time, which a run in a thread of its own gets
// from its system prompt.
func runPrompt(loop Loop, run Run, now time.Time, thread string) string {
	var b strings.Builder
	if thread == ThreadMain {
		fmt.Fprintf(&b, "[routine] %s · %s\n", loop.Name, now.In(location(loop)).Format("Mon 2 Jan 2006 15:04 MST"))
	}
	if run.Reason != "" {
		fmt.Fprintf(&b, "%s\n", run.Reason)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(loop.Prompt)
	switch thread {
	case ThreadMain:
		b.WriteString("\n\nDeliver this routine's outcome with send_message or an app presentation tool unless the routine says to stay quiet. When a tool confirms that Jaz presented its result, that view is already the answer; do not repeat it in a text message.")
	case ThreadChild:
		b.WriteString("\n\nSend what the user should hear from this run with send_message; it reaches the bot that owns this routine, which tells the user. Send nothing when nothing came of it.")
	}
	return b.String()
}

// missedAfter is how late a slot or an event may be and still count as on
// time; anything later was missed while Jaz was not running.
const missedAfter = 15 * time.Minute

// reasonBudget bounds how much event text one run gathers.
const reasonBudget = 20000

const moreEvents = "More matching messages arrived after these; look them up before you act."

// foldReason adds an event that fires a run to the run's reason.
func foldReason(reason, event string) string {
	switch {
	case reason == "":
		return "Triggered by: " + event
	case strings.HasSuffix(reason, moreEvents):
		return reason
	case len(reason)+len(event) > reasonBudget:
		return reason + "\n\n" + moreEvents
	}
	return reason + "\n\n" + event
}

type missed struct {
	count       int
	first, last time.Time
	// due is the slot after the missed ones that is still on time, if any.
	due time.Time
}

// missedSlots walks schedule's slots from first up to now.
func missedSlots(schedule Schedule, first, now time.Time) (missed, error) {
	loc, _, err := scheduleLocation(schedule.Timezone)
	if err != nil {
		return missed{}, err
	}
	parsed, err := cronParser.Parse(schedule.Expr)
	if err != nil {
		return missed{}, err
	}
	result := missed{first: first}
	for slot := first; !slot.IsZero() && !slot.After(now); slot = parsed.Next(slot.In(loc)).UTC() {
		if now.Sub(slot) <= missedAfter {
			result.due = slot
			break
		}
		result.count++
		result.last = slot
	}
	return result, nil
}

func missedText(loop Loop, missed missed) string {
	if missed.count == 1 {
		return "Jaz was not running at " + localTime(loop, missed.first)
	}
	return fmt.Sprintf("Jaz was not running for %d scheduled runs, %s to %s", missed.count, localTime(loop, missed.first), localTime(loop, missed.last))
}

// catchUpReason tells the run that covers missed slots what it covers and
// where the last good run left off, so it does not repeat that run's work.
func catchUpReason(loop Loop, missed missed, lastOK time.Time) string {
	since := "No run of this routine has finished successfully yet."
	if !lastOK.IsZero() {
		since = "The last successful run finished " + localTime(loop, lastOK) + "; cover everything since then in this run."
	}
	return "Catching up: " + missedText(loop, missed) + ". " + since
}

func localTime(loop Loop, at time.Time) string {
	return at.In(location(loop)).Format("Mon 2 Jan 15:04 MST")
}

func location(loop Loop) *time.Location {
	loc, _, err := scheduleLocation(loop.Schedule.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ensureWebhookSecret gives a webhook routine a secret the first time it needs
// one; only its hash is stored.
func ensureWebhookSecret(loop *Loop) {
	if loop.Trigger == nil || loop.Trigger.Kind != TriggerWebhook || loop.WebhookHash != "" {
		return
	}
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	secret := hex.EncodeToString(raw)
	loop.WebhookHash = hashSecret(secret)
	loop.WebhookSecret = secret
}

// VerifyWebhookSecret reports whether secret opens loop's webhook trigger,
// which only an active routine has.
func VerifyWebhookSecret(loop Loop, secret string) bool {
	if loop.Status != StatusActive || loop.Trigger == nil || loop.Trigger.Kind != TriggerWebhook || loop.WebhookHash == "" || secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(loop.WebhookHash)) == 1
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
