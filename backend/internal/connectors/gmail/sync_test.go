package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestKeepMessage(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		keep   bool
	}{
		{"primary no category", []string{"INBOX", "UNREAD"}, true},
		{"personal", []string{"CATEGORY_PERSONAL"}, true},
		{"sent", []string{"SENT"}, true},
		{"promotions", []string{"CATEGORY_PROMOTIONS"}, false},
		{"social", []string{"CATEGORY_SOCIAL"}, false},
		{"forums", []string{"CATEGORY_FORUMS"}, false},
		{"updates", []string{"UNREAD", "CATEGORY_UPDATES"}, false},
		{"important overrides updates", []string{"IMPORTANT", "CATEGORY_UPDATES"}, true},
		{"starred overrides promotions", []string{"STARRED", "CATEGORY_PROMOTIONS"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := keepMessage(tc.labels); got != tc.keep {
				t.Fatalf("keepMessage(%v) = %v, want %v", tc.labels, got, tc.keep)
			}
		})
	}
}

func TestObserveIncrementalDropsNoiseMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/history":
			_, _ = w.Write([]byte(`{"historyId":"h2","history":[{"messagesAdded":[
				{"message":{"id":"promo"}},
				{"message":{"id":"real"}}
			]}]}`))
		case "/gmail/v1/users/me/messages/promo":
			_, _ = w.Write([]byte(`{"id":"promo","labelIds":["CATEGORY_PROMOTIONS"],"payload":{"headers":[{"name":"Subject","value":"Sale"}]}}`))
		case "/gmail/v1/users/me/messages/real":
			_, _ = w.Write([]byte(`{"id":"real","labelIds":["CATEGORY_PERSONAL"],"payload":{"headers":[{"name":"Subject","value":"Hello"}]}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cursor, err := EncodeSyncCursor(SyncCursor{BackfillComplete: true, HistoryID: "h1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).Observe(context.Background(), integrations.ObserveRequest{
		Connection: integrations.Connection{ID: "conn_1", AccountID: "augustinas@example.com"},
		Cursor:     cursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 || result.Records[0].ExternalID != "real" {
		t.Fatalf("records = %#v", result.Records)
	}
}
