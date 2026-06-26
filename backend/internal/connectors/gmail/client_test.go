package gmail

import (
	"context"
	"encoding/base64"
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

func TestAPIClientSearchMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			if r.URL.Query().Get("q") != "from:alice" || r.URL.Query().Get("maxResults") != "2" {
				t.Fatalf("query = %#v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","threadId":"t1"}],"resultSizeEstimate":1}`))
		case "/gmail/v1/users/me/messages/m1":
			if r.URL.Query().Get("format") != "metadata" {
				t.Fatalf("format = %s", r.URL.Query().Get("format"))
			}
			_, _ = w.Write([]byte(`{
				"id":"m1",
				"threadId":"t1",
				"historyId":"h1",
				"snippet":"Snippet text",
				"labelIds":["INBOX","UNREAD"],
				"internalDate":"1710000000000",
				"payload":{"headers":[
					{"name":"Subject","value":"Hello"},
					{"name":"From","value":"Alice <alice@example.com>"},
					{"name":"To","value":"Bob <bob@example.com>"}
				]}
			}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).SearchMessages(context.Background(), SearchMessagesRequest{
		Query:      "from:alice",
		MaxResults: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %#v", result.Messages)
	}
	message := result.Messages[0]
	if message.ID != "m1" || message.ThreadID != "t1" || message.HistoryID != "h1" || message.Subject != "Hello" || message.Snippet != "Snippet text" {
		t.Fatalf("message = %#v", message)
	}
	if len(message.From) != 1 || message.From[0].Name != "Alice" || message.From[0].Email != "alice@example.com" {
		t.Fatalf("from = %#v", message.From)
	}
	if len(message.LabelIDs) != 2 || message.InternalDate.IsZero() {
		t.Fatalf("labels/date = %#v %v", message.LabelIDs, message.InternalDate)
	}
}

func TestAPIClientReadMessage(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("Plain body"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/messages/m1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "full" {
			t.Fatalf("format = %s", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte(`{
			"id":"m1",
			"threadId":"t1",
			"snippet":"Snippet text",
			"payload":{
				"mimeType":"multipart/mixed",
				"headers":[{"name":"Subject","value":"Hello"}],
				"parts":[
					{"mimeType":"text/plain","body":{"data":"` + body + `"}},
					{"mimeType":"application/pdf","filename":"invoice.pdf","body":{"attachmentId":"att1","size":123}}
				]
			}
		}`))
	}))
	defer server.Close()

	content, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).ReadMessage(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if content.Message.ID != "m1" || content.Message.Subject != "Hello" || content.BodyText != "Plain body" {
		t.Fatalf("content = %#v", content)
	}
	if len(content.Message.Attachments) != 1 || content.Message.Attachments[0].ID != "att1" || content.Message.Attachments[0].FileName != "invoice.pdf" || content.Message.Attachments[0].Size != 123 {
		t.Fatalf("attachments = %#v", content.Message.Attachments)
	}
}
