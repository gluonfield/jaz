package connections

import (
	"context"
	"encoding/base64"
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

func TestGmailMCPToolsSearchAndReadMessages(t *testing.T) {
	body := base64.RawURLEncoding.EncodeToString([]byte("Plain body"))
	gmailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			if r.URL.Query().Get("q") != "subject:hello" || r.URL.Query().Get("maxResults") != "5" {
				t.Fatalf("query = %#v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","threadId":"t1"}],"resultSizeEstimate":1}`))
		case "/gmail/v1/users/me/messages/m1":
			switch r.URL.Query().Get("format") {
			case "metadata":
				_, _ = w.Write([]byte(`{
					"id":"m1",
					"threadId":"t1",
					"snippet":"Snippet",
					"payload":{"headers":[{"name":"Subject","value":"Hello"}]}
				}`))
			case "full":
				_, _ = w.Write([]byte(`{
					"id":"m1",
					"threadId":"t1",
					"payload":{
						"headers":[{"name":"Subject","value":"Hello"}],
						"parts":[{"mimeType":"text/plain","body":{"data":"` + body + `"}}]
					}
				}`))
			default:
				t.Fatalf("format = %s", r.URL.Query().Get("format"))
			}
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
			ID:       gmailconnector.OAuthConnectionID,
			Provider: gmailconnector.ProviderID,
			Alias:    "default",
		}},
	})
	tools.apiBaseURL = gmailServer.URL

	_, search, err := tools.SearchMessages(context.Background(), nil, GmailSearchMessagesInput{Query: " subject:hello ", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !search.Connected || search.Query != "subject:hello" || len(search.Messages) != 1 || search.Messages[0].ID != "m1" || search.Messages[0].Subject != "Hello" {
		t.Fatalf("search = %#v", search)
	}
	_, read, err := tools.ReadMessage(context.Background(), nil, GmailReadMessageInput{ID: " m1 "})
	if err != nil {
		t.Fatal(err)
	}
	if !read.Connected || read.Content.Message.ID != "m1" || read.Content.BodyText != "Plain body" {
		t.Fatalf("read = %#v", read)
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
