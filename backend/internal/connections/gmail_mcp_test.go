package connections

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

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
	tools := NewGmailMCPTools(store)
	tools.apiBaseURL = gmailServer.URL

	result, profile, err := tools.GetProfile(context.Background(), nil, GmailProfileInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Connected || profile.EmailAddress != "augustinas@example.com" || profile.MessagesTotal != 42 || profile.ThreadsTotal != 7 || profile.HistoryID != "123" {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.AccountID != "augustinas@example.com" || profile.AccountName != "Augustinas" || profile.Alias != "personal" || len(profile.Scopes) != 1 || profile.Scopes[0] != gmailconnector.ScopeModify {
		t.Fatalf("account fields = %#v", profile)
	}
	if got := gmailToolText(result); !strings.Contains(got, "Gmail is connected as augustinas@example.com") {
		t.Fatalf("text = %q", got)
	}
}

func TestGmailMCPToolsGetProfileReportsNotConnected(t *testing.T) {
	_, profile, err := NewGmailMCPTools(&gmailMCPStore{}).GetProfile(context.Background(), nil, GmailProfileInput{})
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
	_, profile, err := NewGmailMCPTools(store).GetProfile(context.Background(), nil, GmailProfileInput{})
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
						"message":{"id":"reply-draft-message","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Re: Hello"}]}}
					}`))
					return
				}
				if !strings.Contains(message, `To: "Alice" <alice@example.com>`) || !strings.Contains(message, "Subject: Hello") || !strings.Contains(message, "Plain body") {
					t.Fatalf("created draft = %s", message)
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Hello"}]}}
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
					"message":{"id":"draft-message","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Hello"}]}}
				}`))
			case http.MethodPut:
				message := decodedGmailDraftMessage(t, r)
				if !strings.Contains(message, "Updated body") || !strings.Contains(message, "Subject: Hello") {
					t.Fatalf("updated draft = %s", message)
				}
				_, _ = w.Write([]byte(`{
					"id":"draft1",
					"message":{"id":"draft-message-2","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Hello"}]}}
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
				"payload":{"headers":[{"name":"Subject","value":"Hello"}]}
			}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer gmailServer.Close()

	tools := NewGmailMCPTools(&gmailMCPStore{
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
	if !search.Connected || search.Query != "subject:hello" || len(search.Threads) != 1 || search.Threads[0].ID != "t1" || search.Threads[0].Messages[0].MessageID != "<m1@example.com>" {
		t.Fatalf("search = %#v", search)
	}
	_, read, err := tools.ReadThread(context.Background(), nil, GmailReadThreadInput{ID: " m1 "})
	if err != nil {
		t.Fatal(err)
	}
	if !read.Connected || read.Thread.ID != "t1" || len(read.Thread.Messages) != 1 || read.Thread.Messages[0].BodyText != "Thread body" {
		t.Fatalf("read = %#v", read)
	}
	_, draft, err := tools.CreateDraft(context.Background(), nil, GmailCreateDraftInput{
		To:       []string{" Alice <alice@example.com> "},
		Subject:  " Hello ",
		BodyText: "Plain body",
		ThreadID: " t1 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !draft.Connected || draft.Draft.ID != "draft1" || draft.Draft.Message.ThreadID != "t1" {
		t.Fatalf("draft = %#v", draft)
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
	_, drafts, err := tools.ListDrafts(context.Background(), nil, GmailListDraftsInput{Query: " subject:hello ", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !drafts.Connected || len(drafts.Drafts) != 1 || drafts.Drafts[0].ID != "draft1" {
		t.Fatalf("drafts = %#v", drafts)
	}
	bodyText := "Updated body"
	_, updated, err := tools.UpdateDraft(context.Background(), nil, GmailUpdateDraftInput{ID: " draft1 ", BodyText: &bodyText})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Connected || updated.Draft.Message.ID != "draft-message-2" {
		t.Fatalf("updated = %#v", updated)
	}
	_, sent, err := tools.SendDraft(context.Background(), nil, GmailSendDraftInput{ID: " draft1 "})
	if err != nil {
		t.Fatal(err)
	}
	if !sent.Connected || sent.Message.ID != "sent1" || sent.Message.ThreadID != "t1" {
		t.Fatalf("sent = %#v", sent)
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
	tools := NewGmailMCPTools(store)
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
	text := strings.Repeat("t", maxGmailBodyChars+1)
	html := strings.Repeat("h", maxGmailBodyChars+1)

	content := gmailToolContent(gmailconnector.MessageContent{
		Message:  gmailconnector.Message{ID: "m1"},
		BodyText: text,
		BodyHTML: html,
	})
	if content.BodyHTML != "" || content.BodyHTMLTruncated {
		t.Fatalf("html should be omitted when plain text exists: %#v", content)
	}
	if !content.BodyTextTruncated || len([]rune(content.BodyText)) != maxGmailBodyChars+3 {
		t.Fatalf("body text = %d truncated=%v", len([]rune(content.BodyText)), content.BodyTextTruncated)
	}

	fallback := gmailToolContent(gmailconnector.MessageContent{
		Message:  gmailconnector.Message{ID: "m2"},
		BodyHTML: html,
	})
	if fallback.BodyText != "" || fallback.BodyTextTruncated || !fallback.BodyHTMLTruncated || len([]rune(fallback.BodyHTML)) != maxGmailBodyChars+3 {
		t.Fatalf("html fallback = %#v", fallback)
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
