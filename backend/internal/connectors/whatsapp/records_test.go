package whatsapp

import (
	"testing"

	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestMessageRecordDisplayTextUsesProtoJSONQuotedPayload(t *testing.T) {
	payload := protoJSON(t, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			ContextInfo: &waE2E.ContextInfo{
				QuotedMessage: &waE2E.Message{Conversation: proto.String("original message")},
			},
		},
	})
	record := MessageRecord{MessagePayload: payload}

	if got := record.DisplayText(); got != "original message" {
		t.Fatalf("display text = %q", got)
	}
}

func TestMessageRecordDisplayTextUsesProtoJSONWebMessage(t *testing.T) {
	payload := protoJSON(t, &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			ID:        proto.String("web-quoted"),
			RemoteJID: proto.String("12345@s.whatsapp.net"),
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				ContextInfo: &waE2E.ContextInfo{
					QuotedMessage: &waE2E.Message{Conversation: proto.String("web original")},
				},
			},
		},
	})
	record := MessageRecord{WebMessage: payload}

	if got := record.DisplayText(); got != "web original" {
		t.Fatalf("display text = %q", got)
	}
}

func TestMessageQuotedTextExtractsExplicitContextInfo(t *testing.T) {
	message := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			ContextInfo: &waE2E.ContextInfo{
				QuotedMessage: &waE2E.Message{Conversation: proto.String("quoted")},
			},
		},
	}

	if got := MessageQuotedText(message); got != "quoted" {
		t.Fatalf("quoted text = %q", got)
	}
}

func protoJSON(t *testing.T, message proto.Message) []byte {
	t.Helper()
	data, err := protojson.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
