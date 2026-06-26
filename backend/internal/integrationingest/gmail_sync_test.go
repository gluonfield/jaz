package integrationingest

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestGmailSyncerWritesRawMessagesAndCursor(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	body := base64.RawURLEncoding.EncodeToString([]byte("Indexed body"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","threadId":"t1"}]}`))
		case "/gmail/v1/users/me/messages/m1":
			_, _ = w.Write([]byte(`{
				"id":"m1",
				"threadId":"t1",
				"internalDate":"` + jsonNumber(occurred.UnixMilli()) + `",
				"payload":{
					"headers":[{"name":"Subject","value":"Sync me"}],
					"parts":[{"mimeType":"text/plain","body":{"data":"` + body + `"}}]
				}
			}`))
		case "/gmail/v1/users/me/profile":
			_, _ = w.Write([]byte(`{"emailAddress":"augustinas@example.com","historyId":"h2"}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	store := &fakeGmailSyncStore{
		connections: []integrations.Connection{{
			ID:        "gmail:personal",
			Provider:  gmailconnector.ProviderID,
			AccountID: "augustinas@example.com",
			Alias:     "personal",
		}},
		token: integrationoauth.Token{
			AccessToken: "access",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Hour),
		},
	}
	syncer := GmailSyncer{
		Store:           store,
		Writer:          RawWriter{Root: root},
		APIBaseURL:      server.URL,
		MaxPagesPerTick: 1,
	}
	if err := syncer.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	matches, err := filepath.Glob(filepath.Join(root, "gmail", "augustinas-example-com", "gmail-personal", "messages", "2026", "06", "25", "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("raw files = %#v", matches)
	}
	file, err := os.Open(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("expected one raw record")
	}
	var record integrations.Record
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	var content gmailconnector.MessageContent
	if err := json.Unmarshal(record.Raw, &content); err != nil {
		t.Fatal(err)
	}
	if record.ExternalID != "m1" || content.BodyText != "Indexed body" {
		t.Fatalf("record = %#v content = %#v", record, content)
	}
	cursor, err := gmailconnector.DecodeSyncCursor(store.cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !cursor.BackfillComplete || cursor.HistoryID != "h2" {
		t.Fatalf("cursor = %#v", cursor)
	}
}

type fakeGmailSyncStore struct {
	connections []integrations.Connection
	token       integrationoauth.Token
	cursor      integrations.Cursor
}

func (s *fakeGmailSyncStore) ListConnections(context.Context, string) ([]integrations.Connection, error) {
	return s.connections, nil
}

func (s *fakeGmailSyncStore) LoadToken(context.Context, string) (integrationoauth.Token, bool, error) {
	return s.token, s.token.AccessToken != "", nil
}

func (s *fakeGmailSyncStore) SaveToken(_ context.Context, _ string, token integrationoauth.Token) error {
	s.token = token
	return nil
}

func (s *fakeGmailSyncStore) LoadIntegrationCursor(context.Context, string, string) (integrations.Cursor, bool, error) {
	return s.cursor, !s.cursor.Empty(), nil
}

func (s *fakeGmailSyncStore) SaveIntegrationCursor(_ context.Context, _ string, cursor integrations.Cursor) error {
	s.cursor = cursor
	return nil
}

func jsonNumber(value int64) string {
	data, _ := json.Marshal(value)
	return string(data)
}
