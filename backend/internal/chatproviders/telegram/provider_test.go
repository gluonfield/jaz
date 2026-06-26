package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
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
