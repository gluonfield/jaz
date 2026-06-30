package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/sourcepaths"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type TelegramMaterializer struct{}

type telegramContactRaw struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	Phone     string `json:"phone"`
	Title     string `json:"title"`
	Type      string `json:"type"`
}

type telegramMessageRaw struct {
	ID      int    `json:"id"`
	Out     bool   `json:"out"`
	Message string `json:"message"`
	FromID  string `json:"from_id"`
	PeerID  string `json:"peer_id"`
	Kind    string `json:"kind"`
}

func (TelegramMaterializer) SourceTargets(_ context.Context, req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	switch req.Record.Kind {
	case "telegram.contact":
		return telegramContactTargets(req)
	case "telegram.message":
		return telegramMessageTargets(req)
	default:
		return nil, nil
	}
}

func (TelegramMaterializer) ProjectSource(_ context.Context, req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	switch req.Target.Kind {
	case "contact_list":
		return telegramContactsArtifact(req)
	case "chat_day":
		return telegramChatDayArtifact(req)
	default:
		return integrations.Artifact{}, nil
	}
}

func telegramContactTargets(req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	var raw telegramContactRaw
	if err := json.Unmarshal(req.Record.Raw, &raw); err != nil {
		return nil, err
	}
	account := recordAccountSlug(req.Record.AccountID)
	return []integrations.SourceTarget{{
		Provider:  "telegram",
		Kind:      "contact_list",
		PathHint:  sourcepaths.ChatContactPath("telegram", account),
		MediaType: "text/markdown",
		Replay:    sourceReplay(account, integrations.ReplayScope{Domain: integrations.RecordDomainContacts}),
	}}, nil
}

func telegramMessageTargets(req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	var raw telegramMessageRaw
	if err := json.Unmarshal(req.Record.Raw, &raw); err != nil {
		return nil, err
	}
	occurred := recordTime(req.Record)
	if occurred.IsZero() {
		return nil, nil
	}
	kind, id, ok := telegramPeerPath(raw.PeerID)
	if !ok {
		return nil, nil
	}
	conversation := kind + ":" + id
	account := recordAccountSlug(req.Record.AccountID)
	utc := occurred.UTC()
	day := utc.Format("2006-01-02")
	return []integrations.SourceTarget{{
		Provider:    "telegram",
		Kind:        "chat_day",
		PathHint:    sourcepaths.ChatConversationSegmentsDayPath("telegram", account, utc, kind, id),
		MediaType:   "text/markdown",
		Key:         sourceKey(conversation, day),
		Replay:      sourceReplay(account, integrations.ReplayScope{Domain: integrations.RecordDomainMessages, Day: day}, integrations.ReplayScope{Domain: integrations.RecordDomainContacts}),
		ContactRefs: nonEmpty(raw.PeerID, raw.FromID),
	}}, nil
}

func telegramContactsArtifact(req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	var lines []string
	seen := map[string]bool{}
	for _, record := range req.Records {
		if record.Kind != "telegram.contact" {
			continue
		}
		var raw telegramContactRaw
		if err := json.Unmarshal(record.Raw, &raw); err != nil {
			return integrations.Artifact{}, err
		}
		line := "- " + strings.Join(nonEmpty(telegramContactLabel(raw, record.ExternalID)...), " | ")
		if !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return integrations.Artifact{}, nil
	}
	sort.Strings(lines)
	body := "# Telegram contacts\n\n" + strings.Join(lines, "\n") + "\n"
	return sourceArtifact(req.Target, body), nil
}

func telegramChatDayArtifact(req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	contacts := telegramContactIndex(req.Records)
	conversation := req.Target.Key.Entity
	kind, _, _ := strings.Cut(conversation, ":")
	group := kind == "chat" || kind == "channel"
	var lines []chatLine
	for _, record := range req.Records {
		if record.Kind != "telegram.message" {
			continue
		}
		var raw telegramMessageRaw
		if err := json.Unmarshal(record.Raw, &raw); err != nil {
			return integrations.Artifact{}, err
		}
		if raw.PeerID != req.Target.Key.Entity || recordTime(record).UTC().Format("2006-01-02") != req.Target.Key.Day {
			continue
		}
		speaker, info := selfSpeaker, ""
		if !raw.Out {
			speakerID := firstText(raw.FromID, raw.PeerID, "Unknown")
			info = contacts[speakerID]
			speaker = firstText(labelHead(info), speakerID)
		}
		text := oneLine(raw.Message)
		if text == "" {
			text = "[" + firstText(raw.Kind, "service") + "]"
		}
		lines = append(lines, chatLine{At: recordTime(record).UTC(), ExternalID: record.ExternalID, Speaker: speaker, SpeakerInfo: info, Text: text})
	}
	conv := resolveConversation(conversation, lines, group, telegramConversationDisplay(conversation, contacts))
	return chatDayArtifact(req.Target, "Telegram", conv)
}

func telegramPeerPath(peerID string) (string, string, bool) {
	kind, id, ok := strings.Cut(peerID, ":")
	if !ok || id == "" || kind != "user" && kind != "chat" && kind != "channel" {
		return "", "", false
	}
	_, err := strconv.ParseInt(id, 10, 64)
	return kind, id, err == nil
}

func telegramConversationDisplay(peerID string, contacts map[string]string) string {
	if head := labelHead(contacts[peerID]); head != "" {
		return head
	}
	return peerID
}

func telegramContactDisplay(raw telegramContactRaw, externalID string) string {
	name := strings.TrimSpace(strings.Join(nonEmpty(raw.FirstName, raw.LastName), " "))
	fallback := ""
	if key := telegramContactKey(raw, externalID); key != "" {
		fallback = "telegram:" + key
	}
	return firstText(name, raw.Title, telegramHandle(raw), fallback)
}

func telegramHandle(raw telegramContactRaw) string {
	if raw.Username == "" {
		return ""
	}
	return "@" + raw.Username
}

func telegramPhone(raw telegramContactRaw) string {
	if raw.Phone == "" {
		return ""
	}
	return "+" + strings.TrimPrefix(raw.Phone, "+")
}

func telegramContactLabel(raw telegramContactRaw, externalID string) []string {
	external := externalID
	if external == "" && raw.ID != 0 {
		external = telegramContactKey(raw, externalID)
	}
	return nonEmpty(telegramContactDisplay(raw, externalID), external, telegramHandle(raw), telegramPhone(raw))
}

func telegramContactIndex(records []integrations.Record) map[string]string {
	out := map[string]string{}
	for _, record := range records {
		if record.Kind != "telegram.contact" {
			continue
		}
		var raw telegramContactRaw
		if json.Unmarshal(record.Raw, &raw) != nil {
			continue
		}
		label := strings.Join(telegramContactLabel(raw, record.ExternalID), " | ")
		for _, key := range nonEmpty(record.ExternalID, telegramContactKey(raw, record.ExternalID)) {
			out[key] = label
		}
	}
	return out
}

func telegramContactKey(raw telegramContactRaw, externalID string) string {
	if raw.ID == 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", telegramContactKind(raw, externalID), raw.ID)
}

func telegramContactKind(raw telegramContactRaw, externalID string) string {
	if raw.Type != "" {
		return raw.Type
	}
	if kind, _, ok := strings.Cut(externalID, ":"); ok && kind != "" {
		return kind
	}
	return "user"
}

var _ integrations.SourceProjector = TelegramMaterializer{}

func (TelegramMaterializer) SourceProvider() string { return "telegram" }
