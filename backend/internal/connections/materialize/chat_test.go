package materialize

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestTelegramMaterializerCreatesChatDayAndContactSources(t *testing.T) {
	contactRaw, _ := json.Marshal(map[string]any{
		"id":         276369933,
		"first_name": "Majid",
		"last_name":  "Yazdani",
		"username":   "majid",
		"phone":      "447700900123",
	})
	contactRecord := integrations.Record{
		Provider:   "telegram",
		AccountID:  "42",
		Kind:       "telegram.contact",
		ExternalID: "user:276369933",
		Raw:        contactRaw,
	}
	contactArtifact := projectOne(t, TelegramMaterializer{}, contactRecord)
	if contactArtifact.PathHint != "sources/telegram/42/contacts.md" {
		t.Fatalf("contact artifact = %#v", contactArtifact)
	}
	contactBody := string(contactArtifact.Body)
	for _, want := range []string{"Majid Yazdani", "user:276369933", "@majid", "+447700900123"} {
		if !strings.Contains(contactBody, want) {
			t.Fatalf("contact body missing %q:\n%s", want, contactBody)
		}
	}

	occurred := time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC)
	messageRaw, _ := json.Marshal(map[string]any{
		"id":      7,
		"out":     false,
		"message": "are we still on?",
		"from_id": "user:276369933",
		"peer_id": "user:276369933",
	})
	messageArtifact := projectOne(t, TelegramMaterializer{}, integrations.Record{
		Provider:   "telegram",
		AccountID:  "42",
		Kind:       "telegram.message",
		ExternalID: "user:276369933:7",
		OccurredAt: occurred,
		Raw:        messageRaw,
	})
	if !strings.Contains(messageArtifact.PathHint, "sources/telegram/42/conversations/user-276369933-") || !strings.HasSuffix(messageArtifact.PathHint, "/2026/06/27.md") {
		t.Fatalf("message artifact = %#v", messageArtifact)
	}
	if !sameStrings(messageArtifactTargetRefs(t, TelegramMaterializer{}, integrations.Record{
		Provider:   "telegram",
		AccountID:  "42",
		Kind:       "telegram.message",
		ExternalID: "user:276369933:7",
		OccurredAt: occurred,
		Raw:        messageRaw,
	}), []string{"user:276369933"}) {
		t.Fatalf("unexpected telegram contact refs")
	}
	body := string(messageArtifact.Body)
	for _, want := range []string{"# Telegram · user:276369933", "## 2026-06-27 UTC", "10:42:09 user:276369933: are we still on?"} {
		if !strings.Contains(body, want) {
			t.Fatalf("message body missing %q:\n%s", want, body)
		}
	}
}

func TestTelegramProjectSourceResolvesContactNamesInChat(t *testing.T) {
	contactRaw, _ := json.Marshal(map[string]any{
		"id":         276369933,
		"first_name": "Majid",
		"last_name":  "Yazdani",
		"username":   "majid",
		"phone":      "447700900123",
	})
	messageRaw, _ := json.Marshal(map[string]any{
		"id":      7,
		"message": "are we still on?",
		"from_id": "user:276369933",
		"peer_id": "user:276369933",
	})
	message := integrations.Record{
		Provider:   "telegram",
		AccountID:  "42",
		Kind:       "telegram.message",
		ExternalID: "user:276369933:7",
		OccurredAt: time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC),
		Raw:        messageRaw,
	}
	targets, err := (TelegramMaterializer{}).SourceTargets(context.Background(), integrations.MaterializeRequest{Record: message})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := (TelegramMaterializer{}).ProjectSource(context.Background(), integrations.SourceProjectionRequest{
		Target: targets[0],
		Records: []integrations.Record{{
			Provider:   "telegram",
			AccountID:  "42",
			Kind:       "telegram.contact",
			ExternalID: "user:276369933",
			Raw:        contactRaw,
		}, message},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(artifact.Body)
	for _, want := range []string{"Majid Yazdani | user:276369933 | @majid | +447700900123", "10:42:09 Majid Yazdani: are we still on?"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestTelegramProjectSourceKeepsChatAndUserPeerKindsSeparate(t *testing.T) {
	chatRaw, _ := json.Marshal(map[string]any{
		"id":    123,
		"title": "Project Group",
		"type":  "chat",
	})
	messageRaw, _ := json.Marshal(map[string]any{
		"id":      7,
		"message": "hello",
		"from_id": "user:123",
		"peer_id": "chat:999",
	})
	message := integrations.Record{
		Provider:   "telegram",
		AccountID:  "42",
		Kind:       "telegram.message",
		ExternalID: "chat:999:7",
		OccurredAt: time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC),
		Raw:        messageRaw,
	}
	targets, err := (TelegramMaterializer{}).SourceTargets(context.Background(), integrations.MaterializeRequest{Record: message})
	if err != nil {
		t.Fatal(err)
	}
	if !sameStrings(targets[0].ContactRefs, []string{"chat:999", "user:123"}) {
		t.Fatalf("contact refs = %#v", targets[0].ContactRefs)
	}
	artifact, err := (TelegramMaterializer{}).ProjectSource(context.Background(), integrations.SourceProjectionRequest{
		Target: targets[0],
		Records: []integrations.Record{{
			Provider:   "telegram",
			AccountID:  "42",
			Kind:       "telegram.contact",
			ExternalID: "chat:123",
			Raw:        chatRaw,
		}, message},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(artifact.Body)
	if strings.Contains(body, "Project Group: hello") {
		t.Fatalf("chat contact leaked into user speaker:\n%s", body)
	}
	if !strings.Contains(body, "10:42:09 user:123: hello") {
		t.Fatalf("body missing user speaker:\n%s", body)
	}
}

func TestWhatsAppMaterializerCreatesChatDayAndContactSources(t *testing.T) {
	contactRaw, _ := json.Marshal(map[string]any{
		"jid":           "447700900123@s.whatsapp.net",
		"phone_number":  "447700900123",
		"display_name":  "Majid Yazdani",
		"contact_names": []string{"Majid"},
	})
	contactArtifact := projectOne(t, WhatsAppMaterializer{}, integrations.Record{
		Provider:   "whatsapp",
		AccountID:  "15550102222",
		Kind:       "whatsapp.contact",
		ExternalID: "447700900123@s.whatsapp.net",
		Raw:        contactRaw,
	})
	if contactArtifact.PathHint != "sources/whatsapp/15550102222/contacts.md" {
		t.Fatalf("contact artifact = %#v", contactArtifact)
	}
	contactBody := string(contactArtifact.Body)
	for _, want := range []string{"Majid Yazdani", "447700900123@s.whatsapp.net", "447700900123"} {
		if !strings.Contains(contactBody, want) {
			t.Fatalf("contact body missing %q:\n%s", want, contactBody)
		}
	}

	occurred := time.Date(2026, 6, 27, 11, 2, 3, 0, time.UTC)
	messageRaw, _ := json.Marshal(map[string]any{
		"id":        "wamid.1",
		"chat":      "447700900123@s.whatsapp.net",
		"sender":    "447700900123@s.whatsapp.net",
		"from_me":   false,
		"push_name": "Majid",
		"text":      "yes",
	})
	messageArtifact := projectOne(t, WhatsAppMaterializer{}, integrations.Record{
		Provider:   "whatsapp",
		AccountID:  "15550102222",
		Kind:       "whatsapp.message",
		ExternalID: "wamid.1",
		OccurredAt: occurred,
		Raw:        messageRaw,
	})
	if !sameStrings(messageArtifactTargetRefs(t, WhatsAppMaterializer{}, integrations.Record{
		Provider:   "whatsapp",
		AccountID:  "15550102222",
		Kind:       "whatsapp.message",
		ExternalID: "wamid.1",
		OccurredAt: occurred,
		Raw:        messageRaw,
	}), []string{"447700900123@s.whatsapp.net"}) {
		t.Fatalf("unexpected whatsapp contact refs")
	}
	body := string(messageArtifact.Body)
	for _, want := range []string{"# WhatsApp · Majid", "Conversation: Majid (447700900123@s.whatsapp.net)", "## 2026-06-27 UTC", "11:02:03 Majid: yes"} {
		if !strings.Contains(body, want) {
			t.Fatalf("message body missing %q:\n%s", want, body)
		}
	}
}

func TestWhatsAppChatDayNamesPeerWhenOnlyOwnerSpoke(t *testing.T) {
	occurred := time.Date(2025, 7, 17, 2, 39, 15, 0, time.UTC)
	messageRaw, _ := json.Marshal(map[string]any{
		"id":           "3A26D730CDD4A028AF7E",
		"conversation": "447554884345@s.whatsapp.net",
		"remote_jid":   "447554884345@s.whatsapp.net",
		"from_me":      true,
		"text":         "Could you retweet and like, https://x.com/primamente",
	})
	message := integrations.Record{
		Provider:   "whatsapp",
		AccountID:  "447598490355",
		Kind:       "whatsapp.message",
		ExternalID: "3A26D730CDD4A028AF7E",
		OccurredAt: occurred,
		Raw:        messageRaw,
	}

	// No contact known: the peer must still be named by its phone number.
	artifact := projectOne(t, WhatsAppMaterializer{}, message)
	body := string(artifact.Body)
	for _, want := range []string{"# WhatsApp · +447554884345", "- +447554884345", "02:39:15 Me:"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}

	// Contact known: the peer resolves to the contact name.
	contactRaw, _ := json.Marshal(map[string]any{
		"jid":          "447554884345@s.whatsapp.net",
		"phone_number": "447554884345",
		"display_name": "Majid Yazdani",
	})
	targets, err := (WhatsAppMaterializer{}).SourceTargets(context.Background(), integrations.MaterializeRequest{Record: message})
	if err != nil {
		t.Fatal(err)
	}
	named, err := (WhatsAppMaterializer{}).ProjectSource(context.Background(), integrations.SourceProjectionRequest{
		Target: targets[0],
		Records: []integrations.Record{{
			Provider:   "whatsapp",
			AccountID:  "447598490355",
			Kind:       "whatsapp.contact",
			ExternalID: "447554884345@s.whatsapp.net",
			Raw:        contactRaw,
		}, message},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(named.Body), "- Majid Yazdani") {
		t.Fatalf("named body missing peer:\n%s", named.Body)
	}
}

func TestWhatsAppChatDayUsesQuotedTextWhenMessageTextMissing(t *testing.T) {
	occurred := time.Date(2026, 5, 19, 16, 2, 40, 0, time.UTC)
	messageRaw, _ := json.Marshal(map[string]any{
		"id":           "3A884C5DEB7FE1",
		"conversation": "12345-67890@g.us",
		"participant":  "447598490355@s.whatsapp.net",
		"from_me":      true,
		"quoted_text":  "would it be cheating if you used a model to fill in the steps?",
	})
	message := integrations.Record{
		Provider:   "whatsapp",
		AccountID:  "447598490355",
		Kind:       "whatsapp.message",
		ExternalID: "3A884C5DEB7FE1",
		OccurredAt: occurred,
		Raw:        messageRaw,
	}

	artifact := projectOne(t, WhatsAppMaterializer{}, message)
	body := string(artifact.Body)
	if strings.Contains(body, "[message]") {
		t.Fatalf("body still contains placeholder:\n%s", body)
	}
	if !strings.Contains(body, "16:02:40 Me: would it be cheating if you used a model to fill in the steps?") {
		t.Fatalf("body missing quoted text:\n%s", body)
	}
}

func messageArtifactTargetRefs(t *testing.T, projector integrations.SourceProjector, record integrations.Record) []string {
	t.Helper()
	targets, err := projector.SourceTargets(context.Background(), integrations.MaterializeRequest{Record: record})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v", targets)
	}
	return targets[0].ContactRefs
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
