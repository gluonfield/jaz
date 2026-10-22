package acp

import (
	"fmt"

	"github.com/wins/jaz/backend/internal/templates/acpcompletion"
)

// CompletionPrompt hands a finished session's result to its parent, to report
// to recipient.
func CompletionPrompt(job Job, recipient string) string {
	prompt, err := acpcompletion.Render(acpcompletion.Data{
		Slug: job.Slug, Agent: job.ACPAgent, State: job.State, Error: job.Error, Assistant: job.Assistant, Recipient: recipient,
	})
	if err != nil {
		return fmt.Sprintf("ACP session %s (%s) completed a turn with state %s. Reconcile this result with newer instructions and current thread state before updating %s.", job.Slug, job.ACPAgent, job.State, recipient)
	}
	return prompt
}
