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
	"strings"
	"testing"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestGmailSyncerWritesRawMessagesAndCursor(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	body := base64.RawURLEncoding.EncodeToString([]byte(`<html><body>
		<style>.hidden{display:none}</style>
		<p>Indexed body</p>
		<a href="https://click.mailchimp.com/?url=https%3A%2F%2Fexample.com%2Freport%3Futm_source%3Dnews">Read report</a>
		<img src="https://tracker.example.com/open/pixel.png">
		<script>alert("x")</script>
	</body></html>`))
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			if got := r.URL.Query().Get("q"); got != "after:2025/06/26" {
				t.Fatalf("gmail backfill query = %q", got)
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","threadId":"t1"}]}`))
		case "/gmail/v1/users/me/messages/m1":
			_, _ = w.Write([]byte(`{
				"id":"m1",
				"threadId":"t1",
				"internalDate":"` + jsonNumber(occurred.UnixMilli()) + `",
				"payload":{
					"headers":[{"name":"Subject","value":"Sync me"}],
					"parts":[{"mimeType":"text/html","body":{"data":"` + body + `"}}]
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
		Now:             func() time.Time { return now },
	}
	if err := syncer.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	matches, err := filepath.Glob(filepath.Join(root, "gmail", "augustinas-example-com", "messages", "2026", "06", "25", "messages.jsonl"))
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
	var raw map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"provider", "connection_id", "account_id", "kind", "external_id", "occurred_at", "received_at", "raw", "body_html"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("compact export contains %q: %#v", key, raw)
		}
	}
	var message GmailMessageExport
	if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	if message.ID != "m1" ||
		message.ThreadID != "t1" ||
		message.At != "2026-06-25T09:00:00Z" ||
		message.Subject != "Sync me" ||
		!strings.Contains(message.Body, "Indexed body") ||
		!strings.Contains(message.Body, "Read report (https://example.com/report)") {
		t.Fatalf("message = %#v", message)
	}
	for _, unwanted := range []string{"mailchimp", "tracker.example.com", "<img", "script", "utm_source"} {
		if strings.Contains(message.Body, unwanted) {
			t.Fatalf("cleaned export body contains %q:\n%s", unwanted, message.Body)
		}
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
