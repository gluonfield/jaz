package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIClientProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/profile" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"augustinas@example.com","messagesTotal":12,"threadsTotal":3,"historyId":"99"}`))
	}))
	defer server.Close()

	profile, err := APIClient{HTTP: server.Client(), BaseURL: server.URL}.Profile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.EmailAddress != "augustinas@example.com" || profile.MessagesTotal != 12 || profile.ThreadsTotal != 3 || profile.HistoryID != "99" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestAPIClientProfileRequiresEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"historyId":"99"}`))
	}))
	defer server.Close()

	if _, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).Profile(context.Background()); err == nil {
		t.Fatal("expected missing email error")
	}
}

func TestAPIClientNormalizesDisabledAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 403,
				"message": "Gmail API has not been used in project 123456789 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/gmail.googleapis.com/overview?project=123456789 then retry.",
				"status": "PERMISSION_DENIED",
				"errors": [{"reason":"accessNotConfigured"}],
				"details": [{"reason":"SERVICE_DISABLED"}]
			}
		}`))
	}))
	defer server.Close()

	_, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).Profile(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "gmail api is disabled for the OAuth client project") || strings.Contains(text, "123456789") || strings.Contains(text, "console.developers.google.com") {
		t.Fatalf("error = %q", text)
	}
}

func TestAPIClientSearchAndReadThreads(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("Thread body"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/threads":
			if r.URL.Query().Get("q") != "from:alice" || r.URL.Query().Get("maxResults") != "2" {
				t.Fatalf("query = %#v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"threads":[{"id":"t1"}],"resultSizeEstimate":1}`))
		case "/gmail/v1/users/me/threads/t1":
			switch r.URL.Query().Get("format") {
			case "metadata":
				_, _ = w.Write([]byte(`{
					"id":"t1",
					"historyId":"h1",
					"messages":[{
						"id":"m1",
						"threadId":"t1",
						"payload":{"headers":[
							{"name":"Subject","value":"Hello"},
							{"name":"Message-ID","value":"<m1@example.com>"},
							{"name":"References","value":"<root@example.com>"}
						]}
					}]
				}`))
			case "full":
				_, _ = w.Write([]byte(`{
					"id":"t1",
					"historyId":"h1",
					"messages":[{
						"id":"m1",
						"threadId":"t1",
						"payload":{
							"headers":[{"name":"Subject","value":"Hello"}],
							"parts":[{"mimeType":"text/plain","body":{"data":"` + body + `"}}]
						}
					}]
				}`))
			default:
				t.Fatalf("thread format = %s", r.URL.Query().Get("format"))
			}
		case "/gmail/v1/users/me/messages/m1":
			if r.URL.Query().Get("format") != "metadata" {
				t.Fatalf("message format = %s", r.URL.Query().Get("format"))
			}
			_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Hello"}]}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := APIClient{HTTP: server.Client(), BaseURL: server.URL}
	search, err := client.SearchThreads(context.Background(), SearchThreadsRequest{Query: "from:alice", MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Threads) != 1 || search.Threads[0].ID != "t1" || search.Threads[0].Messages[0].MessageID != "<m1@example.com>" || search.Threads[0].Messages[0].References != "<root@example.com>" {
		t.Fatalf("search = %#v", search)
	}
	thread, err := client.ReadThread(context.Background(), ReadThreadRequest{ID: "m1", IDType: IDTypeMessage, MaxMessages: 20})
	if err != nil {
		t.Fatal(err)
	}
	if thread.ID != "t1" || len(thread.Messages) != 1 || thread.Messages[0].BodyText != "Thread body" {
		t.Fatalf("thread = %#v", thread)
	}
}

func TestAPIClientDraftWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/drafts":
			switch r.Method {
			case http.MethodPost:
				message := decodedDraftMessage(t, r)
				for _, want := range []string{
					"To: \"Alice\" <alice@example.com>",
					"Subject: Hello",
					"In-Reply-To: <m1@example.com>",
					"References: <root@example.com> <m1@example.com>",
					"Plain body",
				} {
					if !strings.Contains(message, want) {
						t.Fatalf("created draft missing %q:\n%s", want, message)
					}
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message","threadId":"thread1","payload":{"headers":[{"name":"Subject","value":"Hello"}]}}
				}`))
			case http.MethodGet:
				if r.URL.Query().Get("q") != "subject:Hello" || r.URL.Query().Get("maxResults") != "2" {
					t.Fatalf("query = %#v", r.URL.Query())
				}
				_, _ = w.Write([]byte(`{"drafts":[{"id":"draft1"}],"resultSizeEstimate":1}`))
			default:
				t.Fatalf("method = %s", r.Method)
			}
		case "/gmail/v1/users/me/drafts/draft1":
			switch r.Method {
			case http.MethodGet:
				if r.URL.Query().Get("format") == "full" {
					body := base64.RawURLEncoding.EncodeToString([]byte("Old body"))
					_, _ = w.Write([]byte(`{
						"id":"draft1",
						"message":{
							"id":"draft-message",
							"threadId":"thread1",
							"payload":{
								"headers":[
									{"name":"Subject","value":"Old subject"},
									{"name":"To","value":"Alice <alice@example.com>"},
									{"name":"Message-ID","value":"<draft@example.com>"}
								],
								"parts":[{"mimeType":"text/plain","body":{"data":"` + body + `"}}]
							}
						}
					}`))
					return
				}
				if r.URL.Query().Get("format") != "metadata" {
					t.Fatalf("format = %s", r.URL.Query().Get("format"))
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message","threadId":"thread1","payload":{"headers":[{"name":"Subject","value":"Hello"}]}}
				}`))
			case http.MethodPut:
				message := decodedDraftMessage(t, r)
				if !strings.Contains(message, `To: "Alice" <alice@example.com>`) || !strings.Contains(message, "Subject: Old subject") || !strings.Contains(message, "New body") {
					t.Fatalf("updated draft = %s", message)
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message-2","threadId":"thread1","payload":{"headers":[{"name":"Subject","value":"Old subject"}]}}
				}`))
			default:
				t.Fatalf("method = %s", r.Method)
			}
		case "/gmail/v1/users/me/drafts/send":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var body struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ID != "draft1" {
				t.Fatalf("send draft id = %q", body.ID)
			}
			_, _ = w.Write([]byte(`{
				"id":"sent1",
				"threadId":"thread1",
				"labelIds":["SENT"],
				"payload":{"headers":[{"name":"Subject","value":"Old subject"}]}
			}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := APIClient{HTTP: server.Client(), BaseURL: server.URL}
	draft, err := client.CreateDraft(context.Background(), ComposeMessageRequest{
		ThreadID:   "thread1",
		To:         []string{"Alice <alice@example.com>"},
		Subject:    "Hello",
		BodyText:   "Plain body",
		InReplyTo:  "<m1@example.com>",
		References: "<root@example.com>\n<m1@example.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID != "draft1" || draft.Message.ThreadID != "thread1" || draft.Message.Subject != "Hello" {
		t.Fatalf("draft = %#v", draft)
	}
	drafts, err := client.ListDrafts(context.Background(), ListDraftsRequest{Query: "subject:Hello", MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts.Drafts) != 1 || drafts.Drafts[0].ID != "draft1" {
		t.Fatalf("drafts = %#v", drafts)
	}
	current, err := client.GetDraft(context.Background(), "draft1")
	if err != nil {
		t.Fatal(err)
	}
	if current.BodyText != "Old body" || current.Draft.Message.Subject != "Old subject" {
		t.Fatalf("current draft = %#v", current)
	}
	updated, err := client.UpdateDraft(context.Background(), "draft1", ComposeMessageRequest{
		ThreadID: "thread1",
		To:       []string{"Alice <alice@example.com>"},
		Subject:  "Old subject",
		BodyText: "New body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != "draft1" || updated.Message.ID != "draft-message-2" {
		t.Fatalf("updated = %#v", updated)
	}
	sent, err := client.SendDraft(context.Background(), "draft1")
	if err != nil {
		t.Fatal(err)
	}
	if sent.ID != "sent1" || sent.ThreadID != "thread1" || sent.Subject != "Old subject" {
		t.Fatalf("sent = %#v", sent)
	}
}

func decodedDraftMessage(t *testing.T, r *http.Request) string {
	t.Helper()
	var body struct {
		Message struct {
			Raw      string `json:"raw"`
			ThreadID string `json:"threadId"`
		} `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Message.ThreadID != "thread1" {
		t.Fatalf("thread id = %q", body.Message.ThreadID)
	}
	raw, err := base64.RawURLEncoding.DecodeString(body.Message.Raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
