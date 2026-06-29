package materialize

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const selfSpeaker = "Me"

type chatLine struct {
	At          time.Time
	ExternalID  string
	Speaker     string
	SpeakerInfo string
	Text        string
}

type chatConversation struct {
	ID    string
	Title string
	Peers []string
	Lines []chatLine
}

func chatDayArtifact(target integrations.SourceTarget, provider string, conv chatConversation) (integrations.Artifact, error) {
	if len(conv.Lines) == 0 {
		return integrations.Artifact{}, nil
	}
	lines := conv.Lines
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].At.Equal(lines[j].At) {
			return lines[i].ExternalID < lines[j].ExternalID
		}
		return lines[i].At.Before(lines[j].At)
	})
	title := oneLine(firstText(conv.Title, conv.ID))
	id := oneLine(conv.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · %s\n\n", provider, title)
	if id != "" && id != title {
		fmt.Fprintf(&b, "Conversation: %s (%s)\n\n", title, id)
	} else {
		fmt.Fprintf(&b, "Conversation: %s\n\n", title)
	}
	writeChatParticipants(&b, conv.Peers, lines)
	fmt.Fprintf(&b, "## %s UTC\n", lines[0].At.Format("2006-01-02"))
	for _, line := range lines {
		fmt.Fprintf(&b, "%s %s: %s\n", line.At.Format("15:04:05"), oneLine(line.Speaker), line.Text)
	}
	return sourceArtifact(target, b.String()), nil
}

func writeChatParticipants(b *strings.Builder, peers []string, lines []chatLine) {
	seen := map[string]bool{}
	var values []string
	add := func(value string) {
		value = oneLine(value)
		head := labelHead(value)
		if head == "" || seen[head] {
			return
		}
		seen[head] = true
		values = append(values, value)
	}
	for _, line := range lines {
		add(firstText(line.SpeakerInfo, line.Speaker))
	}
	for _, peer := range peers {
		add(peer)
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

func labelHead(label string) string {
	if label == "" {
		return ""
	}
	return strings.Split(label, " | ")[0]
}

func resolveConversation(id string, lines []chatLine, group bool, fallbackTitle string) chatConversation {
	conv := chatConversation{ID: id, Lines: lines}
	if group {
		conv.Title = fallbackTitle
		return conv
	}
	conv.Title = firstText(directPeerLabel(lines), fallbackTitle)
	conv.Peers = nonEmpty(conv.Title)
	return conv
}

func directPeerLabel(lines []chatLine) string {
	for _, line := range lines {
		if line.Speaker != selfSpeaker {
			return line.Speaker
		}
	}
	return ""
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
