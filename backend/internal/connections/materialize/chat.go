package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type TelegramMaterializer struct{}
type WhatsAppMaterializer struct{}

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

type whatsappContactRaw struct {
	WhatsAppID   string   `json:"whatsapp_id"`
	JID          string   `json:"jid"`
	PhoneNumber  string   `json:"phone_number"`
	Phone        string   `json:"phone"`
	DisplayName  string   `json:"display_name"`
	ContactNames []string `json:"contact_names"`
	FirstName    string   `json:"first_name"`
	FullName     string   `json:"full_name"`
	PushName     string   `json:"push_name"`
	BusinessName string   `json:"business_name"`
}

type whatsappMessageRaw struct {
	ID           string `json:"id"`
	Chat         string `json:"chat"`
	Conversation string `json:"conversation"`
	RemoteJID    string `json:"remote_jid"`
	Sender       string `json:"sender"`
	Participant  string `json:"participant"`
	FromMe       bool   `json:"from_me"`
	IsGroup      bool   `json:"is_group"`
	PushName     string `json:"push_name"`
	MediaType    string `json:"media_type"`
	Text         string `json:"text"`
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

func (WhatsAppMaterializer) SourceTargets(_ context.Context, req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	switch req.Record.Kind {
	case "whatsapp.contact":
		return whatsappContactTargets(req)
	case "whatsapp.message":
		return whatsappMessageTargets(req)
	default:
		return nil, nil
	}
}

func (WhatsAppMaterializer) ProjectSource(_ context.Context, req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	switch req.Target.Kind {
	case "contact_list":
		return whatsappContactsArtifact(req)
	case "chat_day":
		return whatsappChatDayArtifact(req)
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
		PathHint:  path.Join("sources", "telegram", account, "contacts.md"),
		MediaType: "text/markdown",
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
	account := recordAccountSlug(req.Record.AccountID)
	conversation := firstText(raw.PeerID, req.Record.ExternalID)
	utc := occurred.UTC()
	return []integrations.SourceTarget{{
		Provider:    "telegram",
		Kind:        "chat_day",
		PathHint:    path.Join("sources", "telegram", account, "conversations", integrations.SourceSlug(conversation), utc.Format("2006"), utc.Format("01"), utc.Format("02")+".md"),
		MediaType:   "text/markdown",
		ContactRefs: nonEmpty(raw.PeerID, raw.FromID),
	}}, nil
}

func whatsappContactTargets(req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	var raw whatsappContactRaw
	if err := json.Unmarshal(req.Record.Raw, &raw); err != nil {
		return nil, err
	}
	account := recordAccountSlug(req.Record.AccountID)
	return []integrations.SourceTarget{{
		Provider:  "whatsapp",
		Kind:      "contact_list",
		PathHint:  path.Join("sources", "whatsapp", account, "contacts.md"),
		MediaType: "text/markdown",
	}}, nil
}

func whatsappMessageTargets(req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	var raw whatsappMessageRaw
	if err := json.Unmarshal(req.Record.Raw, &raw); err != nil {
		return nil, err
	}
	occurred := recordTime(req.Record)
	if occurred.IsZero() {
		return nil, nil
	}
	account := recordAccountSlug(req.Record.AccountID)
	conversation := firstText(raw.Chat, raw.Conversation, raw.RemoteJID, req.Record.ExternalID)
	utc := occurred.UTC()
	return []integrations.SourceTarget{{
		Provider:    "whatsapp",
		Kind:        "chat_day",
		PathHint:    path.Join("sources", "whatsapp", account, "conversations", integrations.SourceSlug(conversation), utc.Format("2006"), utc.Format("01"), utc.Format("02")+".md"),
		MediaType:   "text/markdown",
		ContactRefs: nonEmpty(raw.Chat, raw.Conversation, raw.RemoteJID, raw.Participant, raw.Sender),
	}}, nil
}

type chatLine struct {
	At          time.Time
	ExternalID  string
	Speaker     string
	SpeakerInfo string
	Text        string
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
	targetSlug, targetDay, ok := chatDayTarget(req.Target.PathHint)
	if !ok {
		return integrations.Artifact{}, fmt.Errorf("unsupported chat source path %q", req.Target.PathHint)
	}
	var conversation string
	var lines []chatLine
	for _, record := range req.Records {
		if record.Kind != "telegram.message" {
			continue
		}
		var raw telegramMessageRaw
		if err := json.Unmarshal(record.Raw, &raw); err != nil {
			return integrations.Artifact{}, err
		}
		recordConversation := firstText(raw.PeerID, record.ExternalID)
		if integrations.SourceSlug(recordConversation) != targetSlug || recordTime(record).UTC().Format("2006-01-02") != targetDay {
			continue
		}
		conversation = firstText(conversation, recordConversation)
		speaker := firstText(raw.FromID, raw.PeerID, "Unknown")
		info := contacts[speaker]
		if raw.Out {
			speaker = "Me"
			info = ""
		} else if info != "" {
			speaker = strings.Split(info, " | ")[0]
		}
		text := oneLine(raw.Message)
		if text == "" {
			text = "[" + firstText(raw.Kind, "service") + "]"
		}
		lines = append(lines, chatLine{At: recordTime(record).UTC(), ExternalID: record.ExternalID, Speaker: speaker, SpeakerInfo: info, Text: text})
	}
	return chatDayArtifact(req.Target, "Telegram", conversation, lines)
}

func whatsappContactsArtifact(req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	var lines []string
	seen := map[string]bool{}
	for _, record := range req.Records {
		if record.Kind != "whatsapp.contact" {
			continue
		}
		var raw whatsappContactRaw
		if err := json.Unmarshal(record.Raw, &raw); err != nil {
			return integrations.Artifact{}, err
		}
		line := "- " + strings.Join(nonEmpty(whatsappContactLabel(raw, record.ExternalID)...), " | ")
		if !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return integrations.Artifact{}, nil
	}
	sort.Strings(lines)
	body := "# WhatsApp contacts\n\n" + strings.Join(lines, "\n") + "\n"
	return sourceArtifact(req.Target, body), nil
}

func whatsappChatDayArtifact(req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	contacts := whatsappContactIndex(req.Records)
	targetSlug, targetDay, ok := chatDayTarget(req.Target.PathHint)
	if !ok {
		return integrations.Artifact{}, fmt.Errorf("unsupported chat source path %q", req.Target.PathHint)
	}
	var conversation string
	var lines []chatLine
	for _, record := range req.Records {
		if record.Kind != "whatsapp.message" {
			continue
		}
		var raw whatsappMessageRaw
		if err := json.Unmarshal(record.Raw, &raw); err != nil {
			return integrations.Artifact{}, err
		}
		recordConversation := firstText(raw.Chat, raw.Conversation, raw.RemoteJID, record.ExternalID)
		if integrations.SourceSlug(recordConversation) != targetSlug || recordTime(record).UTC().Format("2006-01-02") != targetDay {
			continue
		}
		conversation = firstText(conversation, recordConversation)
		senderID := firstText(raw.Participant, raw.Sender, raw.RemoteJID, conversation)
		sender := firstText(raw.PushName, senderID, "Unknown")
		info := contacts[senderID]
		if raw.FromMe {
			sender = "Me"
			info = ""
		} else if info != "" {
			sender = strings.Split(info, " | ")[0]
		}
		text := oneLine(raw.Text)
		if text == "" {
			text = "[message]"
			if raw.MediaType != "" {
				text = "[" + oneLine(raw.MediaType) + "]"
			}
		}
		lines = append(lines, chatLine{At: recordTime(record).UTC(), ExternalID: record.ExternalID, Speaker: sender, SpeakerInfo: info, Text: text})
	}
	return chatDayArtifact(req.Target, "WhatsApp", conversation, lines)
}

func chatDayTarget(sourcePath string) (string, string, bool) {
	clean := path.Clean(sourcePath)
	parts := strings.Split(clean, "/")
	if len(parts) != 8 || parts[0] != "sources" || parts[3] != "conversations" || !strings.HasSuffix(parts[7], ".md") {
		return "", "", false
	}
	return parts[4], parts[5] + "-" + parts[6] + "-" + strings.TrimSuffix(parts[7], ".md"), true
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

func whatsappContactDisplay(raw whatsappContactRaw) string {
	return firstText(raw.DisplayName, raw.FullName, raw.PushName, raw.BusinessName, raw.FirstName, raw.JID, raw.WhatsAppID)
}

func whatsappPhone(raw whatsappContactRaw) string {
	return firstText(raw.PhoneNumber, raw.Phone)
}

func whatsappContactLabel(raw whatsappContactRaw, externalID string) []string {
	jid := firstText(raw.JID, raw.WhatsAppID, externalID)
	values := append([]string{whatsappContactDisplay(raw), jid, whatsappPhone(raw)}, raw.ContactNames...)
	return nonEmpty(values...)
}

func whatsappContactIndex(records []integrations.Record) map[string]string {
	out := map[string]string{}
	for _, record := range records {
		if record.Kind != "whatsapp.contact" {
			continue
		}
		var raw whatsappContactRaw
		if json.Unmarshal(record.Raw, &raw) != nil {
			continue
		}
		label := strings.Join(whatsappContactLabel(raw, record.ExternalID), " | ")
		for _, key := range nonEmpty(record.ExternalID, raw.JID, raw.WhatsAppID, raw.PhoneNumber, raw.Phone) {
			out[key] = label
		}
	}
	return out
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

var (
	_ integrations.SourceProjector = TelegramMaterializer{}
	_ integrations.SourceProjector = WhatsAppMaterializer{}
)

func (TelegramMaterializer) SourceProvider() string { return "telegram" }

func (WhatsAppMaterializer) SourceProvider() string { return "whatsapp" }
