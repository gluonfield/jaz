package materialize

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type WhatsAppMaterializer struct{}

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
		Replay:    sourceReplay(account, integrations.ReplayScope{Domain: integrations.RecordDomainContacts}),
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
	day := utc.Format("2006-01-02")
	return []integrations.SourceTarget{{
		Provider:    "whatsapp",
		Kind:        "chat_day",
		PathHint:    path.Join("sources", "whatsapp", account, "conversations", integrations.SourceSlug(conversation), utc.Format("2006"), utc.Format("01"), utc.Format("02")+".md"),
		MediaType:   "text/markdown",
		Key:         sourceKey(conversation, day),
		Replay:      sourceReplay(account, integrations.ReplayScope{Domain: integrations.RecordDomainMessages, Day: day}, integrations.ReplayScope{Domain: integrations.RecordDomainContacts}),
		ContactRefs: nonEmpty(raw.Chat, raw.Conversation, raw.RemoteJID, raw.Participant, raw.Sender),
	}}, nil
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
		if recordConversation != req.Target.Key.Entity || recordTime(record).UTC().Format("2006-01-02") != req.Target.Key.Day {
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

var _ integrations.SourceProjector = WhatsAppMaterializer{}

func (WhatsAppMaterializer) SourceProvider() string { return "whatsapp" }
