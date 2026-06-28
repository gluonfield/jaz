package connections

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func newTestGmailMCPTools(t *testing.T, store GmailToolStore) *GmailMCPTools {
	t.Helper()
	return NewGmailMCPTools(store, integrationingest.RawWriter{Root: t.TempDir()})
}

func TestGmailMCPToolsGetProfile(t *testing.T) {
	gmailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/profile" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"augustinas@example.com","messagesTotal":42,"threadsTotal":7,"historyId":"123"}`))
	}))
	defer gmailServer.Close()

	store := &gmailMCPStore{
		tokens: map[string]integrationoauth.Token{
			gmailconnector.OAuthConnectionID: {
				AccessToken: "access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
				Scopes:      []string{gmailconnector.ScopeModify},
			},
		},
		connections: []integrations.Connection{{
			ID:          gmailconnector.OAuthConnectionID,
			Provider:    gmailconnector.ProviderID,
			AccountID:   "augustinas@example.com",
			AccountName: "Augustinas",
			Alias:       "personal",
			Scopes:      []string{gmailconnector.ScopeModify},
		}},
	}
	tools := newTestGmailMCPTools(t, store)
	tools.apiBaseURL = gmailServer.URL

	result, profile, err := tools.GetProfile(context.Background(), nil, GmailProfileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Connected || profile.EmailAddress != "augustinas@example.com" || profile.MessagesTotal != 42 || profile.ThreadsTotal != 7 {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.AccountID != "augustinas@example.com" || profile.AccountName != "Augustinas" || profile.Alias != "personal" {
		t.Fatalf("account fields = %#v", profile)
	}
	if got := gmailToolText(result); !strings.Contains(got, "Gmail is connected as augustinas@example.com") {
		t.Fatalf("text = %q", got)
	}
}

func TestGmailMCPToolsGetProfileReportsNotConnected(t *testing.T) {
	_, profile, err := newTestGmailMCPTools(t, &gmailMCPStore{}).GetProfile(context.Background(), nil, GmailProfileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Connected {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestGmailMCPToolsRequiresVerifiedConnection(t *testing.T) {
	store := &gmailMCPStore{
		tokens: map[string]integrationoauth.Token{
			gmailconnector.OAuthConnectionID: {
				AccessToken: "access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
			},
		},
	}
	_, profile, err := newTestGmailMCPTools(t, store).GetProfile(context.Background(), nil, GmailProfileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Connected {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestGmailMCPToolsThreadAndDraftWorkflow(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("Thread body"))
	draftBody := base64.RawURLEncoding.EncodeToString([]byte("Old body"))
	gmailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/gmail/v1/users/me/threads":
			if r.URL.Query().Get("q") != "subject:hello" || r.URL.Query().Get("maxResults") != "5" {
				t.Fatalf("query = %#v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"threads":[{"id":"t1"}],"resultSizeEstimate":1}`))
		case "/gmail/v1/users/me/threads/t1":
			switch r.URL.Query().Get("format") {
			case "metadata":
				_, _ = w.Write([]byte(`{
					"id":"t1",
					"messages":[{
						"id":"m1",
						"threadId":"t1",
						"snippet":"Snippet",
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
					"messages":[{
						"id":"m1",
						"threadId":"t1",
						"payload":{
							"headers":[
								{"name":"Subject","value":"Hello"},
								{"name":"From","value":"Alice <alice@example.com>"},
								{"name":"To","value":"Augustinas <augustinas@example.com>, Bob <bob@example.com>"},
								{"name":"Cc","value":"Carol <carol@example.com>"},
								{"name":"Message-ID","value":"<m1@example.com>"},
								{"name":"References","value":"<root@example.com>"}
							],
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
		case "/gmail/v1/users/me/drafts":
			switch r.Method {
			case http.MethodPost:
				message := decodedGmailDraftMessage(t, r)
				if strings.Contains(message, "Reply body") {
					for _, want := range []string{
						`To: "Alice" <alice@example.com>, "Bob" <bob@example.com>`,
						`Cc: "Carol" <carol@example.com>, "Dan" <dan@example.com>`,
						`Bcc: "Erin" <erin@example.com>`,
						"Subject: Re: Hello",
						"In-Reply-To: <m1@example.com>",
						"References: <root@example.com> <m1@example.com>",
						"Reply body",
					} {
						if !strings.Contains(message, want) {
							t.Fatalf("reply draft missing %q:\n%s", want, message)
						}
					}
					_, _ = w.Write([]byte(`{
						"id":"reply-draft",
						"message":{"id":"reply-draft-message","threadId":"t1","historyId":"h-reply","payload":{"headers":[
							{"name":"Subject","value":"Re: Hello"},
							{"name":"Message-ID","value":"<reply-draft@example.com>"}
						]}}
					}`))
					return
				}
				if !strings.Contains(message, `To: "Alice" <alice@example.com>`) || !strings.Contains(message, "Subject: Hello") || !strings.Contains(message, "Plain body") {
					t.Fatalf("created draft = %s", message)
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message","threadId":"t1","historyId":"h-draft","payload":{"headers":[
						{"name":"Subject","value":"Hello"},
						{"name":"Message-ID","value":"<draft-created@example.com>"}
					]}}
				}`))
			case http.MethodGet:
				if r.URL.Query().Get("q") != "subject:hello" || r.URL.Query().Get("maxResults") != "5" {
					t.Fatalf("draft query = %#v", r.URL.Query())
				}
				_, _ = w.Write([]byte(`{"drafts":[{"id":"draft1"}],"resultSizeEstimate":1}`))
			default:
				t.Fatalf("method = %s", r.Method)
			}
		case "/gmail/v1/users/me/drafts/draft1":
			switch r.Method {
			case http.MethodGet:
				if r.URL.Query().Get("format") == "full" {
					_, _ = w.Write([]byte(`{
						"id":"draft1",
						"message":{
							"id":"draft-message",
							"threadId":"t1",
							"payload":{
								"headers":[
									{"name":"Subject","value":"Hello"},
									{"name":"To","value":"Alice <alice@example.com>"},
									{"name":"Message-ID","value":"<draft@example.com>"}
								],
								"parts":[{"mimeType":"text/plain","body":{"data":"` + draftBody + `"}}]
							}
						}
					}`))
					return
				}
				if r.URL.Query().Get("format") != "metadata" {
					t.Fatalf("draft format = %s", r.URL.Query().Get("format"))
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message","threadId":"t1","historyId":"h-draft","payload":{"headers":[
						{"name":"Subject","value":"Hello"},
						{"name":"Message-ID","value":"<draft-list@example.com>"}
					]}}
				}`))
			case http.MethodPut:
				message := decodedGmailDraftMessage(t, r)
				if !strings.Contains(message, "Updated body") || !strings.Contains(message, "Subject: Hello") {
					t.Fatalf("updated draft = %s", message)
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message-2","threadId":"t1","historyId":"h-updated","payload":{"headers":[
						{"name":"Subject","value":"Hello"},
						{"name":"Message-ID","value":"<draft-updated@example.com>"}
					]}}
				}`))
			default:
				t.Fatalf("method = %s", r.Method)
			}
		case "/gmail/v1/users/me/drafts/send":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var payload struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.ID != "draft1" {
				t.Fatalf("draft id = %q", payload.ID)
			}
			_, _ = w.Write([]byte(`{
				"id":"sent1",
				"threadId":"t1",
				"historyId":"h-sent",
				"payload":{"headers":[
					{"name":"Subject","value":"Hello"},
					{"name":"Message-ID","value":"<sent@example.com>"}
				]}
			}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer gmailServer.Close()

	tools := newTestGmailMCPTools(t, &gmailMCPStore{
		tokens: map[string]integrationoauth.Token{
			gmailconnector.OAuthConnectionID: {
				AccessToken: "access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
			},
		},
		connections: []integrations.Connection{{
			ID:        gmailconnector.OAuthConnectionID,
			Provider:  gmailconnector.ProviderID,
			AccountID: "augustinas@example.com",
			Alias:     "default",
		}},
	})
	tools.apiBaseURL = gmailServer.URL

	_, search, err := tools.SearchThreads(context.Background(), nil, GmailSearchThreadsInput{Query: " subject:hello ", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !search.Connected || search.Query != "subject:hello" || len(search.Threads) != 1 || search.Threads[0].ID != "t1" || search.Threads[0].Messages[0].ID != "m1" {
		t.Fatalf("search = %#v", search)
	}
	if search.Threads[0].Messages[0].MessageID != "" || search.Threads[0].Messages[0].References != "" || search.Threads[0].HistoryID != "" {
		t.Fatalf("search leaked raw header fields: %#v", search.Threads[0])
	}
	_, read, err := tools.ReadThread(context.Background(), nil, GmailReadThreadInput{ID: " m1 "})
	if err != nil {
		t.Fatal(err)
	}
	if !read.Connected || read.Thread.ID != "t1" || len(read.Thread.Messages) != 1 || read.Thread.Messages[0].BodyText != "Thread body" {
		t.Fatalf("read = %#v", read)
	}
	if got := read.Thread.Messages[0].Message; got.MessageID != "" || got.References != "" || got.InReplyTo != "" || got.HistoryID != "" {
		t.Fatalf("read leaked raw header fields: %#v", got)
	}
	_, draft, err := tools.CreateDraft(context.Background(), nil, GmailCreateDraftInput{
		To:       []string{" Alice <alice@example.com> "},
		Subject:  " Hello ",
		BodyText: "Plain body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !draft.Connected || draft.Draft.ID != "draft1" || draft.Draft.Message.ThreadID != "t1" {
		t.Fatalf("draft = %#v", draft)
	}
	if draft.Draft.Message.MessageID != "" || draft.Draft.Message.HistoryID != "" {
		t.Fatalf("draft leaked raw header fields: %#v", draft.Draft.Message)
	}
	_, reply, err := tools.CreateReplyDraft(context.Background(), nil, GmailCreateReplyDraftInput{
		ID:        "m1",
		ReplyMode: "reply_all",
		BodyText:  "Reply body",
		CcAdd:     []string{"Dan <dan@example.com>"},
		BccAdd:    []string{"Erin <erin@example.com>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Connected || reply.Draft.ID != "reply-draft" || reply.Draft.Message.ThreadID != "t1" {
		t.Fatalf("reply draft = %#v", reply)
	}
	if reply.Draft.Message.MessageID != "" || reply.Draft.Message.HistoryID != "" {
		t.Fatalf("reply draft leaked raw header fields: %#v", reply.Draft.Message)
	}
	_, drafts, err := tools.ListDrafts(context.Background(), nil, GmailListDraftsInput{Query: " subject:hello ", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !drafts.Connected || len(drafts.Drafts) != 1 || drafts.Drafts[0].ID != "draft1" {
		t.Fatalf("drafts = %#v", drafts)
	}
	if drafts.Drafts[0].Message.MessageID != "" || drafts.Drafts[0].Message.HistoryID != "" {
		t.Fatalf("draft list leaked raw header fields: %#v", drafts.Drafts[0].Message)
	}
	bodyText := "Updated body"
	_, updated, err := tools.UpdateDraft(context.Background(), nil, GmailUpdateDraftInput{ID: " draft1 ", BodyText: &bodyText})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Connected || updated.Draft.Message.ID != "draft-message-2" {
		t.Fatalf("updated = %#v", updated)
	}
	if updated.Draft.Message.MessageID != "" || updated.Draft.Message.HistoryID != "" {
		t.Fatalf("updated draft leaked raw header fields: %#v", updated.Draft.Message)
	}
	_, sent, err := tools.SendDraft(context.Background(), nil, GmailSendDraftInput{ID: " draft1 "})
	if err != nil {
		t.Fatal(err)
	}
	if !sent.Connected || sent.Message.ID != "sent1" || sent.Message.ThreadID != "t1" {
		t.Fatalf("sent = %#v", sent)
	}
	if sent.Message.MessageID != "" || sent.Message.HistoryID != "" {
		t.Fatalf("sent draft leaked raw header fields: %#v", sent.Message)
	}
}

func TestGmailMCPToolsReadAttachmentStoresFileAndReturnsPreviewOnly(t *testing.T) {
	textBody := base64.RawURLEncoding.EncodeToString([]byte("Attachment text https://example.com/report?utm_source=email"))
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(`<p>Attachment HTML</p><img src="https://tracker.example.com/open/pixel.png"><a href="https://example.com/doc?utm=1">Doc</a>`))
	binaryBody := base64.RawURLEncoding.EncodeToString([]byte{0, 1, 2, 3})
	gmailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages/m1":
			if r.URL.Query().Get("format") != "full" {
				t.Fatalf("message format = %s", r.URL.Query().Get("format"))
			}
			_, _ = w.Write([]byte(`{
				"id":"m1",
				"threadId":"t1",
				"payload":{"parts":[
					{"filename":"note.txt","mimeType":"text/plain","body":{"attachmentId":"a1","size":57}},
					{"filename":"note.html","mimeType":"text/html","body":{"attachmentId":"html","size":120}},
					{"filename":"file.pdf","mimeType":"application/pdf","body":{"attachmentId":"bin","size":4}}
				]}
			}`))
		case "/gmail/v1/users/me/messages/m1/attachments/a1":
			_, _ = w.Write([]byte(`{"data":"` + textBody + `","size":57}`))
		case "/gmail/v1/users/me/messages/m1/attachments/html":
			_, _ = w.Write([]byte(`{"data":"` + htmlBody + `","size":120}`))
		case "/gmail/v1/users/me/messages/m1/attachments/bin":
			_, _ = w.Write([]byte(`{"data":"` + binaryBody + `","size":4}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer gmailServer.Close()

	tools := newTestGmailMCPTools(t, &gmailMCPStore{
		tokens: map[string]integrationoauth.Token{
			gmailconnector.OAuthConnectionID: {
				AccessToken: "access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
			},
		},
		connections: []integrations.Connection{{
			ID:        gmailconnector.OAuthConnectionID,
			Provider:  gmailconnector.ProviderID,
			AccountID: "augustinas@example.com",
			Alias:     "default",
		}},
	})
	tools.apiBaseURL = gmailServer.URL

	_, textAttachment, err := tools.ReadAttachment(context.Background(), nil, GmailReadAttachmentInput{
		MessageID:    "m1",
		AttachmentID: "a1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !textAttachment.Connected || textAttachment.UnsupportedContent || textAttachment.FilePath == "" || !strings.Contains(textAttachment.TextPreview, "Attachment text https://example.com/report") || strings.Contains(textAttachment.TextPreview, "utm_source") {
		t.Fatalf("text attachment = %#v", textAttachment)
	}
	textData, err := os.ReadFile(textAttachment.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(textData), "utm_source=email") {
		t.Fatalf("stored attachment should keep original bytes, got %q", string(textData))
	}

	_, refAttachment, err := tools.ReadAttachment(context.Background(), nil, GmailReadAttachmentInput{
		AttachmentID: "att:gmail/augustinas-example-com/m1/2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !refAttachment.Connected || refAttachment.AttachmentID != "html" || refAttachment.FileName != "note.html" || !strings.Contains(refAttachment.TextPreview, "Attachment HTML") {
		t.Fatalf("ref attachment = %#v", refAttachment)
	}

	if _, _, err := tools.ReadAttachment(context.Background(), nil, GmailReadAttachmentInput{
		Account:      "default",
		AttachmentID: "att:gmail/work-example-com/m1/2",
	}); err == nil || !strings.Contains(err.Error(), "account does not match attachment ref") {
		t.Fatalf("mismatched account err = %v", err)
	}

	_, htmlAttachment, err := tools.ReadAttachment(context.Background(), nil, GmailReadAttachmentInput{
		MessageID:    "m1",
		AttachmentID: "html",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !htmlAttachment.Connected || htmlAttachment.UnsupportedContent || htmlAttachment.FilePath == "" || !strings.Contains(htmlAttachment.TextPreview, "Attachment HTML") || !strings.Contains(htmlAttachment.TextPreview, "Doc (https://example.com/doc)") {
		t.Fatalf("html attachment = %#v", htmlAttachment)
	}
	for _, unwanted := range []string{"<p>", "<img", "tracker.example.com", "utm="} {
		if strings.Contains(htmlAttachment.TextPreview, unwanted) {
			t.Fatalf("html attachment contains %q: %#v", unwanted, htmlAttachment)
		}
	}

	_, binaryAttachment, err := tools.ReadAttachment(context.Background(), nil, GmailReadAttachmentInput{
		MessageID:    "m1",
		AttachmentID: "bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !binaryAttachment.Connected || !binaryAttachment.UnsupportedContent || binaryAttachment.TextPreview != "" || binaryAttachment.FilePath == "" || binaryAttachment.Size != 4 {
		t.Fatalf("binary attachment = %#v", binaryAttachment)
	}
	binaryData, err := os.ReadFile(binaryAttachment.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(binaryData) != string([]byte{0, 1, 2, 3}) {
		t.Fatalf("binary attachment bytes = %#v", binaryData)
	}
}

func TestGmailMCPToolsRequireAccountWhenMultipleAccountsConnected(t *testing.T) {
	gmailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer work-access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/gmail/v1/users/me/profile" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"work@example.com","messagesTotal":2,"threadsTotal":1,"historyId":"h2"}`))
	}))
	defer gmailServer.Close()

	store := &gmailMCPStore{
		tokens: map[string]integrationoauth.Token{
			"gmail:personal": {
				AccessToken: "personal-access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
			},
			"gmail:work": {
				AccessToken: "work-access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
			},
		},
		connections: []integrations.Connection{{
			ID:        "gmail:personal",
			Provider:  gmailconnector.ProviderID,
			AccountID: "personal@example.com",
			Alias:     "personal",
		}, {
			ID:        "gmail:work",
			Provider:  gmailconnector.ProviderID,
			AccountID: "work@example.com",
			Alias:     "work",
		}},
	}
	tools := newTestGmailMCPTools(t, store)
	tools.apiBaseURL = gmailServer.URL

	result, profile, err := tools.GetProfile(context.Background(), nil, GmailProfileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Connected || !profile.AccountRequired || len(profile.Accounts) != 2 {
		t.Fatalf("profile = %#v", profile)
	}
	if got := gmailToolText(result); !strings.Contains(got, "Specify account") || !strings.Contains(got, "personal") || !strings.Contains(got, "work") {
		t.Fatalf("text = %q", got)
	}

	_, profile, err = tools.GetProfile(context.Background(), nil, GmailProfileInput{Account: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Connected || profile.EmailAddress != "work@example.com" || profile.Alias != "work" {
		t.Fatalf("selected profile = %#v", profile)
	}
}

func TestGmailToolContentBoundsBodyOutput(t *testing.T) {
	text := strings.Repeat("text ", maxGmailBodyChars)
	html := strings.Repeat("html ", maxGmailBodyChars)

	content := gmailToolContent(gmailconnector.MessageContent{
		Message:  gmailconnector.Message{ID: "m1"},
		BodyText: text,
		BodyHTML: html,
	})
	if !content.BodyTextTruncated || len([]rune(content.BodyText)) != maxGmailBodyChars+3 {
		t.Fatalf("body text = %d truncated=%v", len([]rune(content.BodyText)), content.BodyTextTruncated)
	}

	fallback := gmailToolContent(gmailconnector.MessageContent{
		Message:  gmailconnector.Message{ID: "m2"},
		BodyHTML: html,
	})
	if !fallback.BodyTextTruncated || len([]rune(fallback.BodyText)) != maxGmailBodyChars+3 {
		t.Fatalf("html fallback should be cleaned into bounded text: %#v", fallback)
	}
}

func TestGmailToolContentCleansHTMLBeforeReturningToAgent(t *testing.T) {
	content := gmailToolContent(gmailconnector.MessageContent{
		Message: gmailconnector.Message{ID: "m1"},
		BodyHTML: `<p>Hello</p>
			<a href="https://click.mailchimp.com/?url=https%3A%2F%2Fexample.com%2Fdoc%3Futm_source%3Dmail">Read</a>
			<a href="https://example.com/inline.png"><img src="cid:image001"></a>
			<img src="https://tracker.example.com/open/pixel.png">`,
	})
	if !strings.Contains(content.BodyText, "Hello") || !strings.Contains(content.BodyText, "Read (https://example.com/doc)") {
		t.Fatalf("clean body missing useful content: %#v", content)
	}
	for _, unwanted := range []string{"mailchimp", "inline.png", "tracker.example.com", "<img", "utm_source"} {
		if strings.Contains(content.BodyText, unwanted) {
			t.Fatalf("clean body contains %q: %#v", unwanted, content)
		}
	}
}

func decodedGmailDraftMessage(t *testing.T, r *http.Request) string {
	t.Helper()
	var payload struct {
		Message struct {
			Raw string `json:"raw"`
		} `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload.Message.Raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type gmailMCPStore struct {
	tokens      map[string]integrationoauth.Token
	connections []integrations.Connection
}

func (s *gmailMCPStore) LoadToken(_ context.Context, connectionID string) (integrationoauth.Token, bool, error) {
	token, ok := s.tokens[connectionID]
	return token, ok, nil
}

func (s *gmailMCPStore) SaveToken(_ context.Context, connectionID string, token integrationoauth.Token) error {
	if s.tokens == nil {
		s.tokens = map[string]integrationoauth.Token{}
	}
	s.tokens[connectionID] = token
	return nil
}

func (s *gmailMCPStore) ListConnections(_ context.Context, provider string) ([]integrations.Connection, error) {
	var out []integrations.Connection
	for _, connection := range s.connections {
		if connection.Provider == provider {
			out = append(out, connection)
		}
	}
	return out, nil
}

func gmailToolText(result *mcp.CallToolResult) string {
	var out string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			out += text.Text
		}
	}
	return out
}
