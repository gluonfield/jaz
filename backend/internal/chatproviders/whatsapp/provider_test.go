package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	waStore "go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestRecipientJIDParsesPhoneNumbersAndJIDs(t *testing.T) {
	jid, err := recipientJID("+1 (555) 010-2222")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := jid.String(), "15550102222@s.whatsapp.net"; got != want {
		t.Fatalf("phone JID = %q, want %q", got, want)
	}
	jid, err = recipientJID("12345-67890@g.us")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := jid.String(), "12345-67890@g.us"; got != want {
		t.Fatalf("explicit JID = %q, want %q", got, want)
	}
}

func TestConnectionIDsUsesDurableConnectionRows(t *testing.T) {
	ids := connectionIDs([]integrations.Connection{
		{ID: "whatsapp:15550101111"},
		{ID: ""},
		{ID: "whatsapp:15550102222"},
	})
	if !ids["whatsapp:15550101111"] || !ids["whatsapp:15550102222"] || ids[""] {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestNewWhatsAppClientUsesBrowserQRIdentity(t *testing.T) {
	restoreWhatsAppVersion(t)
	version := waStore.WAVersionContainer{2, 3000, 1042247318}
	waStore.SetWAVersion(version)

	client := newWhatsAppClient(&waStore.Device{})
	if client.QRClientType != whatsmeow.PairClientChrome {
		t.Fatalf("QR client type = %q, want %q", client.QRClientType, whatsmeow.PairClientChrome)
	}
	if got := waStore.DeviceProps.GetPlatformType(); got != waCompanionReg.DeviceProps_CHROME {
		t.Fatalf("platform type = %s, want %s", got, waCompanionReg.DeviceProps_CHROME)
	}
	if got := waStore.DeviceProps.GetOs(); got != whatsappCompanionName {
		t.Fatalf("device os = %q, want %q", got, whatsappCompanionName)
	}
	propsVersion := waStore.DeviceProps.GetVersion()
	if got := [3]uint32{propsVersion.GetPrimary(), propsVersion.GetSecondary(), propsVersion.GetTertiary()}; got != version {
		t.Fatalf("device props version = %v, want %v", got, version)
	}
}

func TestConfigureWhatsAppDevicePropsTracksRefreshedVersion(t *testing.T) {
	restoreWhatsAppVersion(t)
	waStore.SetWAVersion(waStore.WAVersionContainer{2, 3000, 1042000000})
	configureWhatsAppDeviceProps()

	version := waStore.WAVersionContainer{2, 3000, 1042247318}
	waStore.SetWAVersion(version)
	configureWhatsAppDeviceProps()

	if got := waStore.DeviceProps.GetOs(); got != whatsappCompanionName {
		t.Fatalf("device os = %q, want %q", got, whatsappCompanionName)
	}
	propsVersion := waStore.DeviceProps.GetVersion()
	if got := [3]uint32{propsVersion.GetPrimary(), propsVersion.GetSecondary(), propsVersion.GetTertiary()}; got != version {
		t.Fatalf("device props version = %v, want %v", got, version)
	}
}

func restoreWhatsAppVersion(t *testing.T) {
	t.Helper()
	original := waStore.GetWAVersion()
	originalOS := waStore.DeviceProps.GetOs()
	originalPlatform := waStore.DeviceProps.GetPlatformType()
	originalPropsVersion := waStore.DeviceProps.GetVersion()
	propsVersion := [3]uint32{
		originalPropsVersion.GetPrimary(),
		originalPropsVersion.GetSecondary(),
		originalPropsVersion.GetTertiary(),
	}
	t.Cleanup(func() {
		waStore.SetWAVersion(original)
		waStore.DeviceProps.Os = proto.String(originalOS)
		waStore.DeviceProps.PlatformType = originalPlatform.Enum()
		waStore.DeviceProps.Version = &waCompanionReg.DeviceProps_AppVersion{
			Primary:   proto.Uint32(propsVersion[0]),
			Secondary: proto.Uint32(propsVersion[1]),
			Tertiary:  proto.Uint32(propsVersion[2]),
		}
	})
}

func TestParseWhatsAppWebVersionFromServiceWorker(t *testing.T) {
	version, err := parseWhatsAppWebVersion([]byte(`SiteData\":{\"client_revision\":1042205873,\"push_phase\":\"C3\"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := version.String(), "2.3000.1042205873"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
	version, err = parseWhatsAppWebVersion([]byte(`{"client_revision":1042205874}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := version.String(), "2.3000.1042205874"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}

func TestFailQRSessionPreservesFailedStatusUntilPolled(t *testing.T) {
	provider := &Provider{sessions: map[string]*qrSession{}}
	session := &qrSession{
		id:     "whatsapp_qr_1",
		status: "pending",
		ready:  make(chan struct{}),
	}
	provider.sessions[session.id] = session

	provider.failQRSession(context.Background(), session, errors.New("save failed"))

	if _, ok := provider.sessions[session.id]; !ok {
		t.Fatalf("session %q was removed before status was read", session.id)
	}
	status, err := provider.QRStatus(context.Background(), session.id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "failed" || status.Error != "save failed" {
		t.Fatalf("status = %#v", status)
	}
	if _, ok := provider.sessions[session.id]; ok {
		t.Fatalf("session %q was not removed after terminal status", session.id)
	}
}

func TestWhatsAppFirstQRCodeErrorPreservesProviderFailure(t *testing.T) {
	err := whatsappFirstQRCodeError(connections.QRStatus{
		Status: "failed",
		Error:  "connect failed",
	})
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestWatchQRScannedWithoutMultideviceSurfacesMessage(t *testing.T) {
	provider := &Provider{}
	session := &qrSession{id: "whatsapp_qr_1", status: "pending", ready: make(chan struct{})}
	qrChan := make(chan whatsmeow.QRChannelItem, 1)
	qrChan <- whatsmeow.QRChannelScannedWithoutMultidevice
	close(qrChan)

	provider.watchQR(session, qrChan)

	status := session.statusSnapshot()
	if status.Status != "scanned" || !strings.Contains(status.Error, "linked-device support") {
		t.Fatalf("status = %#v", status)
	}
}

func TestWatchQRPairingErrorWithoutDetailsHasUsefulMessage(t *testing.T) {
	provider := &Provider{}
	session := &qrSession{id: "whatsapp_qr_1", status: "pending", ready: make(chan struct{})}
	qrChan := make(chan whatsmeow.QRChannelItem, 1)
	qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventError}
	close(qrChan)

	provider.watchQR(session, qrChan)

	status := session.statusSnapshot()
	if status.Status != "failed" || status.Error != "WhatsApp pairing failed" {
		t.Fatalf("status = %#v", status)
	}
}

func TestWhatsAppRecordsExposeContactsAndMessages(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	contact := whatsappContactRecord(connection, waTypes.NewJID("15550103333", waTypes.DefaultUserServer), waTypes.ContactInfo{
		FullName: "Alice Example",
		PushName: "Alice",
	})
	if got, want := contact.Kind.Domain(), integrations.RecordDomainContacts; got != want {
		t.Fatalf("contact domain = %q, want %q", got, want)
	}
	var contactRaw map[string]any
	if err := json.Unmarshal(contact.Raw, &contactRaw); err != nil {
		t.Fatal(err)
	}
	if contactRaw["full_name"] != "Alice Example" ||
		contactRaw["display_name"] != "Alice Example" ||
		contactRaw["phone_number"] != "15550103333" ||
		contactRaw["whatsapp_id"] != "15550103333@s.whatsapp.net" {
		t.Fatalf("contact raw = %#v", contactRaw)
	}
	names, ok := contactRaw["contact_names"].([]any)
	if !ok || len(names) != 2 || names[0] != "Alice Example" || names[1] != "Alice" {
		t.Fatalf("contact names = %#v", contactRaw["contact_names"])
	}

	occurred := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	message := whatsappMessageRecord(connection, &events.Message{
		Info: waTypes.MessageInfo{
			ID: "wamid.1",
			MessageSource: waTypes.MessageSource{
				Chat:   waTypes.NewJID("15550103333", waTypes.DefaultUserServer),
				Sender: waTypes.NewJID("15550103333", waTypes.DefaultUserServer),
			},
			Timestamp: occurred,
			PushName:  "Alice",
		},
		Message: &waE2E.Message{Conversation: proto.String("hello")},
	})
	if got, want := message.Kind.Domain(), integrations.RecordDomainMessages; got != want {
		t.Fatalf("message domain = %q, want %q", got, want)
	}
	if !message.OccurredAt.Equal(occurred) {
		t.Fatalf("message occurred_at = %s, want %s", message.OccurredAt, occurred)
	}
	var messageRaw map[string]any
	if err := json.Unmarshal(message.Raw, &messageRaw); err != nil {
		t.Fatal(err)
	}
	if messageRaw["text"] != "hello" || messageRaw["sender"] != "15550103333@s.whatsapp.net" {
		t.Fatalf("message raw = %#v", messageRaw)
	}
}

func TestWhatsAppGroupRecordCarriesSubjectAsContact(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	jid := waTypes.NewJID("120363409748040236", waTypes.GroupServer)
	group := whatsappGroupRecord(connection, jid, "Enterprise x Employee AI")

	if got, want := group.Kind.Domain(), integrations.RecordDomainContacts; got != want {
		t.Fatalf("group domain = %q, want %q", got, want)
	}
	if group.ExternalID != jid.String() {
		t.Fatalf("group external id = %q, want %q", group.ExternalID, jid.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(group.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["display_name"] != "Enterprise x Employee AI" || raw["jid"] != jid.String() || raw["phone"] != nil {
		t.Fatalf("group raw = %#v", raw)
	}
}

func TestWhatsAppMessageRecordExtractsQuotedText(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	message := whatsappMessageRecord(connection, &events.Message{
		Info: waTypes.MessageInfo{
			ID: "wamid.quoted",
			MessageSource: waTypes.MessageSource{
				Chat:   waTypes.NewJID("15550103333", waTypes.DefaultUserServer),
				Sender: waTypes.NewJID("15550103333", waTypes.DefaultUserServer),
			},
			Timestamp: time.Date(2026, 6, 26, 12, 5, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				ContextInfo: &waE2E.ContextInfo{
					QuotedMessage: &waE2E.Message{Conversation: proto.String("original message")},
				},
			},
		},
	})

	var raw map[string]any
	if err := json.Unmarshal(message.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["text"] != "" || raw["quoted_text"] != "original message" {
		t.Fatalf("message raw = %#v", raw)
	}
}

func TestWriteRecordsMarksSyncCursor(t *testing.T) {
	raw := &fakeWhatsAppRawSink{}
	store := &fakeWhatsAppStore{}
	provider := &Provider{raw: raw, store: store}
	record := integrations.Record{
		Provider:     "whatsapp",
		AccountID:    "15550102222",
		ConnectionID: "whatsapp:15550102222",
		Kind:         "whatsapp.message",
		ExternalID:   "wamid.1",
		Raw:          json.RawMessage(`{"text":"hello"}`),
	}

	if err := provider.writeRecords(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if len(raw.records) != 1 || raw.records[0].ExternalID != record.ExternalID {
		t.Fatalf("records = %#v", raw.records)
	}
	if store.cursorConnectionID != record.ConnectionID || store.cursor.Kind != whatsappSyncCursorKind {
		t.Fatalf("cursor connection=%q cursor=%#v", store.cursorConnectionID, store.cursor)
	}
}

func TestWhatsAppHistoryRecordsDropOldMessagesAndFullProtoBlob(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	newTimestamp := uint64(now.Add(-time.Hour).Unix())
	oldTimestamp := uint64(now.Add(-(whatsappHistoricalWindow + time.Hour)).Unix())
	syncType := waHistorySync.HistorySync_INITIAL_BOOTSTRAP
	chunk := uint32(2)
	progress := uint32(50)
	conversationID := "15550103333@s.whatsapp.net"
	sync := &waHistorySync.HistorySync{
		SyncType:   &syncType,
		ChunkOrder: &chunk,
		Progress:   &progress,
		Conversations: []*waHistorySync.Conversation{{
			ID: proto.String(conversationID),
			Messages: []*waHistorySync.HistorySyncMsg{
				{Message: whatsappWebInfo("new-id", conversationID, newTimestamp, "new message")},
				{Message: whatsappWebInfo("old-id", conversationID, oldTimestamp, "old message")},
			},
		}},
	}

	records := whatsappHistoryRecords(connection, sync, whatsappHistoryCutoff(now))

	if len(records) != 2 {
		t.Fatalf("records len = %d, want metadata + one message", len(records))
	}
	if records[0].Kind != "whatsapp.history" {
		t.Fatalf("metadata kind = %q", records[0].Kind)
	}
	var metadata map[string]any
	if err := json.Unmarshal(records[0].Raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata["conversations"]; ok {
		t.Fatalf("metadata contains full conversations blob: %#v", metadata)
	}
	if records[1].ExternalID != "new-id" {
		t.Fatalf("message external id = %q", records[1].ExternalID)
	}
	var message map[string]any
	if err := json.Unmarshal(records[1].Raw, &message); err != nil {
		t.Fatal(err)
	}
	if message["text"] != "new message" {
		t.Fatalf("message raw = %#v", message)
	}
}

func TestWhatsAppHistorySyncCapsGroupHistoryAcrossChunks(t *testing.T) {
	connection := integrations.Connection{ID: "whatsapp:alice", AccountID: "15550102222"}
	raw := &fakeWhatsAppRawSink{}
	provider := &Provider{
		root:  t.TempDir(),
		raw:   raw,
		cfg:   Config{GroupHistoryLimit: 2},
		store: &fakeWhatsAppStore{},
	}
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	groupID := "12345-67890@g.us"
	first := whatsappHistorySync(groupID, now, 3, "first")
	second := whatsappHistorySync(groupID, now.Add(time.Minute), 5, "second")

	if err := provider.writeHistorySync(context.Background(), connection, first, now); err != nil {
		t.Fatal(err)
	}
	if len(raw.records) != 3 {
		t.Fatalf("first records len = %d, want metadata + 2 messages", len(raw.records))
	}
	if err := provider.writeHistorySync(context.Background(), connection, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(raw.records) != 4 {
		t.Fatalf("second chunk wrote group messages after cap: records len = %d", len(raw.records))
	}
}

func TestPublishHistorySyncDeliversWaitingReadMessages(t *testing.T) {
	chat := "15550103333@s.whatsapp.net"
	provider := &Provider{historyWaiters: map[string][]chan []whatsappconnector.ReadRecentMessage{}}
	waiter := provider.addHistoryWaiter(chat)
	defer provider.removeHistoryWaiter(chat, waiter)

	provider.publishHistorySync(whatsappHistorySync(chat, time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC), 2, "live"))

	select {
	case messages := <-waiter:
		if len(messages) != 2 {
			t.Fatalf("messages len = %d", len(messages))
		}
		if messages[0].MessageID != "live-1" || messages[1].MessageID != "live-0" {
			t.Fatalf("messages = %#v", messages)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history messages")
	}
}

func TestReadRecentReadsStoredRawMessages(t *testing.T) {
	rawRoot := t.TempDir()
	connection := integrations.Connection{
		ID:        "whatsapp:alice",
		AccountID: "15550102222",
	}
	chat := "15550103333@s.whatsapp.net"
	writeWhatsAppRawRecord(t, rawRoot, whatsappWebMessageRecordForTest(connection, chat, "stored-old", time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC), "old"))
	writeWhatsAppRawRecord(t, rawRoot, whatsappWebMessageRecordForTest(connection, chat, "stored-new", time.Date(2026, 6, 26, 12, 1, 0, 0, time.UTC), "new"))
	writeWhatsAppRawRecord(t, rawRoot, whatsappWebMessageRecordForTest(connection, "15550104444@s.whatsapp.net", "other-chat", time.Date(2026, 6, 26, 12, 2, 0, 0, time.UTC), "other"))
	provider := &Provider{
		cfg: Config{RawRoot: rawRoot},
	}

	result, err := provider.ReadRecent(context.Background(), whatsappconnector.ReadRecentRequest{
		Connection: connection,
		Chat:       chat,
		Limit:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Chat != chat || len(result.Messages) != 1 || result.Messages[0].MessageID != "stored-new" {
		t.Fatalf("result = %#v", result)
	}
}

func TestReadRecentUsesStoredMessagesBeforeLiveDuplicates(t *testing.T) {
	rawRoot := t.TempDir()
	connection := integrations.Connection{
		ID:        "whatsapp:alice",
		AccountID: "15550102222",
	}
	chat := "15550103333@s.whatsapp.net"
	storedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	liveAt := time.Date(2026, 6, 26, 12, 1, 0, 0, time.UTC)
	writeWhatsAppRawRecord(t, rawRoot, whatsappWebMessageRecordForTest(connection, chat, "stored-only", storedAt, "stored"))
	writeWhatsAppRawRecord(t, rawRoot, whatsappWebMessageRecordForTest(connection, chat, "live-dup", liveAt, "duplicate"))
	provider := &Provider{
		cfg:            Config{RawRoot: rawRoot},
		recentMessages: map[string][]whatsappconnector.ReadRecentMessage{},
	}
	provider.storeLiveMessages(chat, []whatsappconnector.ReadRecentMessage{{
		MessageID: "live-dup",
		SentAt:    liveAt,
		Text:      "live duplicate",
	}})

	result, err := provider.ReadRecent(context.Background(), whatsappconnector.ReadRecentRequest{
		Connection: connection,
		Chat:       chat,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("messages len = %d, messages = %#v", len(result.Messages), result.Messages)
	}
	if result.Messages[0].MessageID != "stored-only" || result.Messages[1].MessageID != "live-dup" {
		t.Fatalf("messages = %#v", result.Messages)
	}
	if result.Messages[1].Text != "duplicate" {
		t.Fatalf("duplicate should keep stored message text, messages = %#v", result.Messages)
	}
}

func whatsappHistorySync(conversationID string, start time.Time, count int, prefix string) *waHistorySync.HistorySync {
	syncType := waHistorySync.HistorySync_INITIAL_BOOTSTRAP
	messages := make([]*waHistorySync.HistorySyncMsg, 0, count)
	for i := range count {
		messages = append(messages, &waHistorySync.HistorySyncMsg{
			Message: whatsappWebInfo(fmt.Sprintf("%s-%d", prefix, i), conversationID, uint64(start.Add(-time.Duration(i)*time.Second).Unix()), prefix),
		})
	}
	return &waHistorySync.HistorySync{
		SyncType: &syncType,
		Conversations: []*waHistorySync.Conversation{{
			ID:       proto.String(conversationID),
			Messages: messages,
		}},
	}
}

func whatsappWebMessageRecordForTest(connection integrations.Connection, chat, id string, timestamp time.Time, text string) integrations.Record {
	return whatsappWebMessageRecordForTestWithMessage(connection, chat, id, timestamp, &waE2E.Message{Conversation: proto.String(text)})
}

func whatsappWebMessageRecordForTestWithMessage(connection integrations.Connection, chat, id string, timestamp time.Time, message *waE2E.Message) integrations.Record {
	record, ok := whatsappWebMessageRecord(connection, chat, &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			ID:        proto.String(id),
			RemoteJID: proto.String(chat),
		},
		MessageTimestamp: proto.Uint64(uint64(timestamp.Unix())),
		Message:          message,
	})
	if !ok {
		panic("failed to build whatsapp web message record")
	}
	return record
}

func writeWhatsAppRawRecord(t *testing.T, root string, record integrations.Record) {
	t.Helper()
	writer := integrationingest.RawWriter{Root: root, Now: func() time.Time {
		return recordTime(record)
	}}
	if err := writer.WriteRecords(context.Background(), []integrations.Record{record}); err != nil {
		t.Fatal(err)
	}
}

func whatsappWebInfo(id, remoteJID string, timestamp uint64, text string) *waWeb.WebMessageInfo {
	return &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			ID:        proto.String(id),
			RemoteJID: proto.String(remoteJID),
		},
		MessageTimestamp: proto.Uint64(timestamp),
		Message:          &waE2E.Message{Conversation: proto.String(text)},
	}
}

type fakeWhatsAppRawSink struct {
	records []integrations.Record
}

func (s *fakeWhatsAppRawSink) WriteRecords(_ context.Context, records []integrations.Record) error {
	s.records = append(s.records, records...)
	return nil
}

type fakeWhatsAppStore struct {
	cursorConnectionID string
	cursor             integrations.Cursor
}

func (s *fakeWhatsAppStore) ListConnections(context.Context, string) ([]integrations.Connection, error) {
	return nil, nil
}

func (s *fakeWhatsAppStore) SaveConnection(context.Context, integrations.Connection) error {
	return nil
}

func (s *fakeWhatsAppStore) SaveIntegrationCursor(_ context.Context, connectionID string, cursor integrations.Cursor) error {
	s.cursorConnectionID = connectionID
	s.cursor = cursor
	return nil
}
