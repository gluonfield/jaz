package materialize

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type chatLine struct {
	At          time.Time
	ExternalID  string
	Speaker     string
	SpeakerInfo string
	Text        string
}

func chatDayArtifact(target integrations.SourceTarget, provider, conversation string, lines []chatLine) (integrations.Artifact, error) {
	if len(lines) == 0 {
		return integrations.Artifact{}, nil
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].At.Equal(lines[j].At) {
			return lines[i].ExternalID < lines[j].ExternalID
		}
		return lines[i].At.Before(lines[j].At)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "# %s conversation %s\n\n", provider, oneLine(conversation))
	fmt.Fprintf(&b, "Conversation: %s\n\n", oneLine(conversation))
	writeChatParticipants(&b, lines)
	fmt.Fprintf(&b, "## %s UTC\n", lines[0].At.Format("2006-01-02"))
	for _, line := range lines {
		fmt.Fprintf(&b, "%s %s: %s\n", line.At.Format("15:04:05"), oneLine(line.Speaker), line.Text)
	}
	return sourceArtifact(target, b.String()), nil
}

func writeChatParticipants(b *strings.Builder, lines []chatLine) {
	seen := map[string]bool{}
	var values []string
	for _, line := range lines {
		value := firstText(line.SpeakerInfo, line.Speaker)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	if len(values) == 0 {
		return
	}
	sort.Strings(values)
	b.WriteString("Participants:\n")
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
	b.WriteByte('\n')
}

func sourceArtifact(target integrations.SourceTarget, body string) integrations.Artifact {
	return integrations.Artifact{
		Provider:  target.Provider,
		Kind:      target.Kind,
		PathHint:  target.PathHint,
		MediaType: sourceMediaType(target.MediaType),
		Body:      []byte(body),
	}
}

func recordTime(record integrations.Record) time.Time {
	if !record.OccurredAt.IsZero() {
		return record.OccurredAt
	}
	return record.ReceivedAt
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = oneLine(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func DefaultSourceProjectors() []integrations.SourceProjector {
	return []integrations.SourceProjector{
		GmailMaterializer{},
		TelegramMaterializer{},
		WhatsAppMaterializer{},
	}
}
