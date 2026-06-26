package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	defaultGmailSearchLimit = 10
	maxGmailSearchLimit     = 20
	maxGmailBodyChars       = 16000
)

type GmailToolStore interface {
	integrationoauth.Store
	ListConnections(context.Context, string) ([]integrations.Connection, error)
}

type GmailMCPTools struct {
	store      GmailToolStore
	apiBaseURL string
}

type gmailToolSession struct {
	api        gmailconnector.APIClient
	connection integrations.Connection
}

type GmailProfileInput struct{}

type GmailSearchMessagesInput struct {
	Query      string `json:"query,omitempty" jsonschema:"Gmail search query, using Gmail search operators; omit for recent messages"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum messages to return, 1-20; defaults to 10"`
}

type GmailReadMessageInput struct {
	ID string `json:"id" jsonschema:"Gmail message id returned by the Gmail search messages tool"`
}

type GmailProfileOutput struct {
	Connected     bool     `json:"connected"`
	EmailAddress  string   `json:"email_address,omitempty"`
	MessagesTotal int64    `json:"messages_total,omitempty"`
	ThreadsTotal  int64    `json:"threads_total,omitempty"`
	HistoryID     string   `json:"history_id,omitempty"`
	AccountID     string   `json:"account_id,omitempty"`
	AccountName   string   `json:"account_name,omitempty"`
	Alias         string   `json:"alias,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
}

type GmailSearchMessagesOutput struct {
	Connected          bool                     `json:"connected"`
	Query              string                   `json:"query,omitempty"`
	Messages           []gmailconnector.Message `json:"messages,omitempty"`
	ResultSizeEstimate int64                    `json:"result_size_estimate,omitempty"`
	NextPageToken      string                   `json:"next_page_token,omitempty"`
}

type GmailReadMessageOutput struct {
	Connected bool                `json:"connected"`
	Content   GmailMessageContent `json:"content,omitempty"`
}

type GmailMessageContent struct {
	Message           gmailconnector.Message `json:"message"`
	BodyText          string                 `json:"body_text,omitempty"`
	BodyTextTruncated bool                   `json:"body_text_truncated,omitempty"`
	BodyHTML          string                 `json:"body_html,omitempty"`
	BodyHTMLTruncated bool                   `json:"body_html_truncated,omitempty"`
}

func NewGmailMCPTools(store GmailToolStore) *GmailMCPTools {
	return &GmailMCPTools{store: store}
}

func (t *GmailMCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolGetProfile,
		Title:       "Get Gmail profile",
		Description: "Check the connected Gmail email account and return the live Gmail profile totals. This verifies account access; it does not read email message contents.",
	}, t.GetProfile)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolSearchMessages,
		Title:       "Search Gmail messages",
		Description: "Search or list Gmail email messages and return bounded metadata, snippets, labels, and message IDs. Use the Gmail read message tool to read one email result.",
	}, t.SearchMessages)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolReadMessage,
		Title:       "Read Gmail message",
		Description: "Read one Gmail email message by ID, returning headers, labels, attachments, and a bounded body text or HTML fallback.",
	}, t.ReadMessage)
}

func (t *GmailMCPTools) RemoveFrom(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(gmailconnector.ToolGetProfile, gmailconnector.ToolSearchMessages, gmailconnector.ToolReadMessage)
	}
}

func (t *GmailMCPTools) GetProfile(ctx context.Context, _ *mcp.CallToolRequest, _ GmailProfileInput) (*mcp.CallToolResult, GmailProfileOutput, error) {
	session, connected, err := t.session(ctx)
	if err != nil {
		return nil, GmailProfileOutput{}, err
	}
	if !connected {
		out := GmailProfileOutput{Connected: false}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	profile, err := session.api.Profile(ctx)
	if err != nil {
		return nil, GmailProfileOutput{}, err
	}
	out := GmailProfileOutput{
		Connected:     true,
		EmailAddress:  profile.EmailAddress,
		MessagesTotal: profile.MessagesTotal,
		ThreadsTotal:  profile.ThreadsTotal,
		HistoryID:     profile.HistoryID,
		AccountID:     session.connection.AccountID,
		AccountName:   session.connection.AccountName,
		Alias:         session.connection.Alias,
		Scopes:        session.connection.Scopes,
	}
	text := fmt.Sprintf("Gmail is connected as %s. Profile reports %d messages and %d threads.", profile.EmailAddress, profile.MessagesTotal, profile.ThreadsTotal)
	return textResult(text), out, nil
}

func (t *GmailMCPTools) SearchMessages(ctx context.Context, _ *mcp.CallToolRequest, input GmailSearchMessagesInput) (*mcp.CallToolResult, GmailSearchMessagesOutput, error) {
	query := strings.TrimSpace(input.Query)
	session, connected, err := t.session(ctx)
	if err != nil {
		return nil, GmailSearchMessagesOutput{}, err
	}
	if !connected {
		out := GmailSearchMessagesOutput{Connected: false, Query: query}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	search, err := session.api.SearchMessages(ctx, gmailconnector.SearchMessagesRequest{
		Query:      query,
		MaxResults: gmailSearchLimit(input.MaxResults),
	})
	if err != nil {
		return nil, GmailSearchMessagesOutput{}, err
	}
	out := GmailSearchMessagesOutput{
		Connected:          true,
		Query:              query,
		Messages:           search.Messages,
		ResultSizeEstimate: search.ResultSizeEstimate,
		NextPageToken:      search.NextPageToken,
	}
	text := fmt.Sprintf("Found %d Gmail messages.", len(search.Messages))
	if query != "" {
		text = fmt.Sprintf("Found %d Gmail messages for %q.", len(search.Messages), query)
	}
	return textResult(text), out, nil
}

func (t *GmailMCPTools) ReadMessage(ctx context.Context, _ *mcp.CallToolRequest, input GmailReadMessageInput) (*mcp.CallToolResult, GmailReadMessageOutput, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, GmailReadMessageOutput{}, errors.New("id is required")
	}
	session, connected, err := t.session(ctx)
	if err != nil {
		return nil, GmailReadMessageOutput{}, err
	}
	if !connected {
		out := GmailReadMessageOutput{Connected: false}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	content, err := session.api.ReadMessage(ctx, id)
	if err != nil {
		return nil, GmailReadMessageOutput{}, err
	}
	subject := content.Message.Subject
	if subject == "" {
		subject = id
	}
	return textResult("Read Gmail message: " + subject), GmailReadMessageOutput{Connected: true, Content: gmailToolContent(content)}, nil
}

func (t *GmailMCPTools) session(ctx context.Context) (gmailToolSession, bool, error) {
	connection, ok, err := t.defaultConnection(ctx)
	if err != nil {
		return gmailToolSession{}, false, err
	} else if !ok {
		return gmailToolSession{}, false, nil
	}
	client, err := (integrationoauth.Refresher{Store: t.store}).Client(ctx, gmailconnector.OAuthConnectionID)
	if errors.Is(err, integrationoauth.ErrTokenNotFound) {
		return gmailToolSession{}, false, nil
	}
	if err != nil {
		return gmailToolSession{}, false, err
	}
	return gmailToolSession{
		api:        gmailconnector.APIClient{HTTP: client, BaseURL: t.apiBaseURL},
		connection: connection,
	}, true, nil
}

func (t *GmailMCPTools) defaultConnection(ctx context.Context) (integrations.Connection, bool, error) {
	connections, err := t.store.ListConnections(ctx, gmailconnector.ProviderID)
	if err != nil {
		return integrations.Connection{}, false, err
	}
	for _, connection := range connections {
		if connection.ID == gmailconnector.OAuthConnectionID {
			return connection, true, nil
		}
	}
	return integrations.Connection{}, false, nil
}

func gmailSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultGmailSearchLimit
	}
	return min(limit, maxGmailSearchLimit)
}

func gmailToolContent(content gmailconnector.MessageContent) GmailMessageContent {
	text, textTruncated := clampGmailBody(content.BodyText)
	html := ""
	htmlTruncated := false
	if text == "" {
		html, htmlTruncated = clampGmailBody(content.BodyHTML)
	}
	return GmailMessageContent{
		Message:           content.Message,
		BodyText:          text,
		BodyTextTruncated: textTruncated,
		BodyHTML:          html,
		BodyHTMLTruncated: htmlTruncated,
	}
}

func clampGmailBody(body string) (string, bool) {
	if body == "" {
		return "", false
	}
	runes := []rune(body)
	if len(runes) <= maxGmailBodyChars {
		return body, false
	}
	return string(runes[:maxGmailBodyChars]) + "...", true
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
