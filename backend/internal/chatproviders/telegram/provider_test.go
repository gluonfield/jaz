package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestInputPeerFromResolvedUser(t *testing.T) {
	user := &tg.User{ID: 42, FirstName: "Alice"}
	user.SetAccessHash(99)
	resolved := &tg.ContactsResolvedPeer{
		Peer:  &tg.PeerUser{UserID: 42},
		Users: []tg.UserClass{user},
	}
	peer, conversationID, err := inputPeerFromResolved(resolved)
	if err != nil {
		t.Fatal(err)
	}
	userPeer, ok := peer.(*tg.InputPeerUser)
	if !ok {
		t.Fatalf("peer = %T, want *tg.InputPeerUser", peer)
	}
	if userPeer.UserID != 42 || userPeer.AccessHash != 99 || conversationID != "user:42" {
		t.Fatalf("resolved peer = %#v, conversation = %q", userPeer, conversationID)
	}
}

func TestExplicitTelegramPeerRecipients(t *testing.T) {
	provider := &Provider{}
	peer, conversationID, err := provider.resolvePeer(nil, nil, "channel:100:200")
	if err != nil {
		t.Fatal(err)
	}
	channel, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		t.Fatalf("peer = %T, want *tg.InputPeerChannel", peer)
	}
	if channel.ChannelID != 100 || channel.AccessHash != 200 || conversationID != "channel:100" {
		t.Fatalf("resolved channel = %#v, conversation = %q", channel, conversationID)
	}
}

func TestTelegramFirstQRCodeErrorPreservesProviderFailure(t *testing.T) {
	err := telegramFirstQRCodeError(connections.QRStatus{
		Status: "failed",
		Error:  "auth export failed",
	})
	if err == nil || !strings.Contains(err.Error(), "auth export failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestTelegramPasswordNeededRecognizesWrappedRPCError(t *testing.T) {
	err := fmt.Errorf("import: %w", tgerr.New(401, "SESSION_PASSWORD_NEEDED"))
	if !telegramPasswordNeeded(err) {
		t.Fatalf("password needed not recognized from %v", err)
	}
}

func TestTelegramClientUsesDesktopDeviceIdentity(t *testing.T) {
	device := telegramClientDevice()
	if device.DeviceModel != "Desktop" || device.SystemVersion != "Windows 10" || device.LangPack != "tdesktop" {
		t.Fatalf("device = %#v", device)
	}
	if device.AppVersion == "" {
		t.Fatal("missing app version")
	}
}

func TestQRSessionPasswordSubmission(t *testing.T) {
	session := &qrSession{status: "password_required", passwords: make(chan string, 1)}

	if err := session.submitPassword(" secret "); err != nil {
		t.Fatal(err)
	}
	select {
	case password := <-session.passwords:
		if password != " secret " {
			t.Fatalf("password = %q", password)
		}
	default:
		t.Fatal("password was not queued")
	}
	status := session.statusSnapshot()
	if status.Status != "scanned" || status.Error != "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestQRSessionPasswordSubmissionRequiresPasswordState(t *testing.T) {
	session := &qrSession{status: "pending", passwords: make(chan string, 1)}

	err := session.submitPassword("secret")
	if !errors.Is(err, connections.ErrQRPasswordNotRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestQRSessionPasswordRequiredDoesNotExpireQRCode(t *testing.T) {
	session := &qrSession{
		status:    "password_required",
		expiresAt: time.Now().Add(-time.Minute),
	}

	status := session.statusSnapshot()
	if status.Status != "password_required" {
		t.Fatalf("status = %#v", status)
	}
}

func TestBackfillStatePersistsResumePeer(t *testing.T) {
	provider := &Provider{root: t.TempDir()}
	state := backfillState{
		CurrentPeer:         &telegramPeerRef{Kind: "user", ID: 43, AccessHash: 99},
		CurrentPeerOffsetID: 7,
		CompletedPeers:      map[string]bool{"chat:100": true},
		PausedUntil:         time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
	}
	if err := provider.saveBackfillState("telegram:test", state); err != nil {
		t.Fatal(err)
	}
	loaded, err := provider.loadBackfillState("telegram:test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentPeer == nil || loaded.CurrentPeer.key() != "user:43" || loaded.CurrentPeerOffsetID != 7 || !loaded.CompletedPeers["chat:100"] {
		t.Fatalf("loaded state = %#v", loaded)
	}
	peer, ok := loaded.CurrentPeer.inputPeer().(*tg.InputPeerUser)
	if !ok {
		t.Fatalf("peer = %T, want *tg.InputPeerUser", loaded.CurrentPeer.inputPeer())
	}
	if peer.UserID != 43 || peer.AccessHash != 99 {
		t.Fatalf("peer = %#v", peer)
	}
}

func TestBackfillBudgetPausePersistsAcrossRestarts(t *testing.T) {
	provider := &Provider{root: t.TempDir()}
	before := time.Now().UTC()
	err := provider.handleBackfillPause("telegram:test", backfillState{CompletedPeers: map[string]bool{}}, errBackfillBudget)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := provider.loadBackfillState("telegram:test")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.PausedUntil.After(before) {
		t.Fatalf("paused_until = %s, want after %s", loaded.PausedUntil, before)
	}
	if loaded.PausedUntil.After(before.Add(telegramBackfillRunInterval + time.Minute)) {
		t.Fatalf("paused_until = %s, want near one run interval", loaded.PausedUntil)
	}
}

func TestContactsSyncMarkerSuppressesRecentRefresh(t *testing.T) {
	provider := &Provider{root: t.TempDir()}
	if provider.contactsSyncSuppressed("telegram:test") {
		t.Fatal("missing marker should not count as recently synced")
	}
	if err := provider.markContactsSyncSuppressed("telegram:test"); err != nil {
		t.Fatal(err)
	}
	if !provider.contactsSyncSuppressed("telegram:test") {
		t.Fatal("recent marker should suppress contact refresh")
	}
}

func TestDisconnectCancelsClientAndRemovesLocalSessionFiles(t *testing.T) {
	root := t.TempDir()
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		<-runCtx.Done()
		close(done)
	}()
	provider := &Provider{
		root:    root,
		clients: map[string]clientRun{"telegram:test": {cancel: cancel, done: done}},
	}
	connection := integrations.Connection{ID: "telegram:test"}
	for _, path := range []string{
		provider.sessionPath(connection.ID),
		provider.backfillMarkerPath(connection.ID),
		provider.backfillStatePath(connection.ID),
		provider.contactsMarkerPath(connection.ID),
	} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := provider.Disconnect(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runCtx.Done():
	default:
		t.Fatal("disconnect did not cancel the live client context")
	}
	if provider.client(connection.ID) != nil {
		t.Fatal("client still registered after disconnect")
	}
	for _, path := range []string{
		provider.sessionPath(connection.ID),
		provider.backfillMarkerPath(connection.ID),
		provider.backfillStatePath(connection.ID),
		provider.contactsMarkerPath(connection.ID),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file %s still present, stat err=%v", path, err)
		}
	}
}

func TestClearClientPreservesReplacement(t *testing.T) {
	original := &tgclient.Client{}
	replacement := &tgclient.Client{}
	provider := &Provider{
		clients: map[string]clientRun{"telegram:test": {client: replacement}},
	}

	provider.clearClient("telegram:test", original)

	if provider.client("telegram:test") != replacement {
		t.Fatal("clearClient removed a newer client registration")
	}
}

func TestCloseQRPendingSessionCancelsLoginClient(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		<-runCtx.Done()
		close(done)
	}()
	provider := &Provider{
		root:     t.TempDir(),
		clients:  map[string]clientRun{"telegram:test": {cancel: cancel, done: done}},
		sessions: map[string]*qrSession{},
	}
	session := &qrSession{id: "qr_1", connectionID: "telegram:test", status: "pending"}
	provider.sessions[session.id] = session

	if err := provider.CloseQR(context.Background(), session.id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runCtx.Done():
	default:
		t.Fatal("close QR did not cancel pending login client")
	}
	if _, ok := provider.sessions[session.id]; ok {
		t.Fatal("QR session still registered")
	}
	if _, ok := provider.clients[session.connectionID]; ok {
		t.Fatal("pending login client still registered")
	}
}

func TestCloseQRConnectedSessionPreservesClient(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	provider := &Provider{
		root:     t.TempDir(),
		clients:  map[string]clientRun{"telegram:test": {cancel: cancel}},
		sessions: map[string]*qrSession{},
	}
	session := &qrSession{id: "qr_1", connectionID: "telegram:test", status: "connected"}
	provider.sessions[session.id] = session

	if err := provider.CloseQR(context.Background(), session.id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runCtx.Done():
		t.Fatal("close QR canceled a connected client")
	default:
	}
	if _, ok := provider.sessions[session.id]; ok {
		t.Fatal("QR session still registered")
	}
	if _, ok := provider.clients[session.connectionID]; !ok {
		t.Fatal("connected client was removed")
	}
}

func TestFailedQRStatusRemovesLoginFileAfterStatusRead(t *testing.T) {
	provider := &Provider{
		root:     t.TempDir(),
		clients:  map[string]clientRun{},
		sessions: map[string]*qrSession{},
	}
	session := &qrSession{
		id:           "qr_1",
		connectionID: "telegram:test",
		status:       "failed",
		err:          "save failed",
	}
	provider.sessions[session.id] = session
	if err := os.WriteFile(provider.sessionPath(session.connectionID), []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := provider.QRStatus(context.Background(), session.id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "failed" || status.Error != "save failed" {
		t.Fatalf("status = %#v", status)
	}
	if _, ok := provider.sessions[session.id]; ok {
		t.Fatal("failed QR session still registered after status read")
	}
	if _, err := os.Stat(provider.sessionPath(session.connectionID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session file still present, stat err=%v", err)
	}
}

func TestTelegramRecordsExposeContactsAndMessages(t *testing.T) {
	connection := integrations.Connection{ID: "telegram:test", AccountID: "42"}
	user := &tg.User{ID: 43, FirstName: "Alice", LastName: "Example", Phone: "15550103333"}
	user.SetAccessHash(99)
	user.SetUsername("alice")
	records := telegramPeerRecords(connection, map[int64]*tg.User{43: user}, nil, nil)
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if got, want := records[0].Kind.Domain(), integrations.RecordDomainContacts; got != want {
		t.Fatalf("contact domain = %q, want %q", got, want)
	}
	var contactRaw map[string]any
	if err := json.Unmarshal(records[0].Raw, &contactRaw); err != nil {
		t.Fatal(err)
	}
	if contactRaw["username"] != "alice" || contactRaw["phone"] != "15550103333" {
		t.Fatalf("contact raw = %#v", contactRaw)
	}

	messageRecords := telegramMessageRecords(connection, []tg.MessageClass{&tg.Message{
		ID:      7,
		Date:    1782475200,
		Message: "hello",
		PeerID:  &tg.PeerUser{UserID: 43},
	}})
	if len(messageRecords) != 1 {
		t.Fatalf("message records len = %d, want 1", len(messageRecords))
	}
	if got, want := messageRecords[0].Kind.Domain(), integrations.RecordDomainMessages; got != want {
		t.Fatalf("message domain = %q, want %q", got, want)
	}
	if got, want := messageRecords[0].ExternalID, "user:43:7"; got != want {
		t.Fatalf("message external id = %q, want %q", got, want)
	}
	var messageRaw map[string]any
	if err := json.Unmarshal(messageRecords[0].Raw, &messageRaw); err != nil {
		t.Fatal(err)
	}
	if messageRaw["message"] != "hello" {
		t.Fatalf("message raw = %#v", messageRaw)
	}
}

func TestWriteRecordsMarksSyncCursor(t *testing.T) {
	raw := &fakeTelegramRawSink{}
	store := &fakeTelegramStore{}
	provider := &Provider{raw: raw, store: store}
	connection := integrations.Connection{ID: "telegram:test", AccountID: "42"}
	record := integrations.Record{
		Provider:     "telegram",
		AccountID:    "42",
		ConnectionID: connection.ID,
		Kind:         "telegram.message",
		ExternalID:   "user:43:7",
		Raw:          json.RawMessage(`{"message":"hello"}`),
	}

	if err := provider.writeRecords(context.Background(), connection, []integrations.Record{record}); err != nil {
		t.Fatal(err)
	}
	if len(raw.records) != 1 || raw.records[0].ExternalID != record.ExternalID {
		t.Fatalf("records = %#v", raw.records)
	}
	if store.cursorConnectionID != connection.ID || store.cursor.Kind != telegramSyncCursorKind {
		t.Fatalf("cursor connection=%q cursor=%#v", store.cursorConnectionID, store.cursor)
	}
}

func TestTelegramHistoricalBackfillKeepsOnlyOneYearByDefault(t *testing.T) {
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	cutoff := telegramBackfillCutoff(now)
	messages := []tg.MessageClass{
		&tg.Message{ID: 1, Date: cutoff + 60, Message: "inside window"},
		&tg.Message{ID: 2, Date: cutoff - 60, Message: "too old"},
		&tg.MessageService{ID: 3, Date: cutoff, Action: &tg.MessageActionEmpty{}},
	}

	filtered := telegramMessagesSince(messages, cutoff)

	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(filtered))
	}
	if telegramMessageID(filtered[0]) != 1 || telegramMessageID(filtered[1]) != 3 {
		t.Fatalf("filtered messages = %#v", filtered)
	}
	if !messagesReachedCutoff(messages, cutoff) {
		t.Fatal("expected page to stop at historical cutoff")
	}
}

type fakeTelegramRawSink struct {
	records []integrations.Record
}

func (s *fakeTelegramRawSink) WriteRecords(_ context.Context, records []integrations.Record) error {
	s.records = append(s.records, records...)
	return nil
}

type fakeTelegramStore struct {
	cursorConnectionID string
	cursor             integrations.Cursor
}

func (s *fakeTelegramStore) ListConnections(context.Context, string) ([]integrations.Connection, error) {
	return nil, nil
}

func (s *fakeTelegramStore) SaveConnection(context.Context, integrations.Connection) error {
	return nil
}

func (s *fakeTelegramStore) SaveIntegrationCursor(_ context.Context, connectionID string, cursor integrations.Cursor) error {
	s.cursorConnectionID = connectionID
	s.cursor = cursor
	return nil
}
