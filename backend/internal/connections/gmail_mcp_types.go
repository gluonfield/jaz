package connections

import (
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type GmailProfileInput struct {
	Account string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
}

type GmailSearchThreadsInput struct {
	Account    string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	Query      string `json:"query,omitempty" jsonschema:"Gmail search query, using Gmail search operators; omit for recent messages"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum threads to return, 1-20; defaults to 10"`
}

type GmailReadThreadInput struct {
	Account     string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	ID          string `json:"id" jsonschema:"Gmail message id or thread id"`
	IDType      string `json:"id_type,omitempty" jsonschema:"message or thread; defaults to message"`
	MaxMessages int    `json:"max_messages,omitempty" jsonschema:"maximum thread messages to return, 1-50; defaults to 20 most recent messages"`
}

type GmailCreateDraftInput struct {
	Account    string   `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	To         []string `json:"to" jsonschema:"recipient email addresses"`
	Cc         []string `json:"cc,omitempty" jsonschema:"CC recipient email addresses"`
	Bcc        []string `json:"bcc,omitempty" jsonschema:"BCC recipient email addresses"`
	Subject    string   `json:"subject,omitempty" jsonschema:"email subject"`
	BodyText   string   `json:"body_text" jsonschema:"plain text email body"`
	ThreadID   string   `json:"thread_id,omitempty" jsonschema:"Gmail thread id when creating a reply draft in an existing conversation"`
	InReplyTo  string   `json:"in_reply_to,omitempty" jsonschema:"Message-ID header of the message being replied to"`
	References string   `json:"references,omitempty" jsonschema:"References header for the thread being replied to"`
}

type GmailSendDraftInput struct {
	Account string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	ID      string `json:"id" jsonschema:"Gmail draft id"`
}

type GmailUpdateDraftInput struct {
	Account    string   `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	ID         string   `json:"id" jsonschema:"Gmail draft id"`
	To         []string `json:"to,omitempty" jsonschema:"replacement recipient email addresses; omit to preserve"`
	Cc         []string `json:"cc,omitempty" jsonschema:"replacement CC recipient email addresses; omit to preserve"`
	Bcc        []string `json:"bcc,omitempty" jsonschema:"replacement BCC recipient email addresses; omit to preserve"`
	Subject    *string  `json:"subject,omitempty" jsonschema:"replacement email subject; omit to preserve"`
	BodyText   *string  `json:"body_text,omitempty" jsonschema:"replacement plain text email body; omit to preserve"`
	ThreadID   *string  `json:"thread_id,omitempty" jsonschema:"replacement Gmail thread id; omit to preserve"`
	InReplyTo  *string  `json:"in_reply_to,omitempty" jsonschema:"replacement In-Reply-To header; omit to preserve"`
	References *string  `json:"references,omitempty" jsonschema:"replacement References header; omit to preserve"`
}

type GmailListDraftsInput struct {
	Account    string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	Query      string `json:"query,omitempty" jsonschema:"Gmail draft search query; omit for recent drafts"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum drafts to return, 1-20; defaults to 10"`
}

type GmailProfileOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	EmailAddress    string                    `json:"email_address,omitempty"`
	MessagesTotal   int64                     `json:"messages_total,omitempty"`
	ThreadsTotal    int64                     `json:"threads_total,omitempty"`
	HistoryID       string                    `json:"history_id,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	AccountName     string                    `json:"account_name,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
	Scopes          []string                  `json:"scopes,omitempty"`
}

type GmailSearchThreadsOutput struct {
	Connected          bool                      `json:"connected"`
	AccountRequired    bool                      `json:"account_required,omitempty"`
	Accounts           []integrations.Connection `json:"accounts,omitempty"`
	AccountID          string                    `json:"account_id,omitempty"`
	Alias              string                    `json:"alias,omitempty"`
	Query              string                    `json:"query,omitempty"`
	Threads            []gmailconnector.Thread   `json:"threads,omitempty"`
	ResultSizeEstimate int64                     `json:"result_size_estimate,omitempty"`
	NextPageToken      string                    `json:"next_page_token,omitempty"`
}

type GmailReadThreadOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
	Thread          GmailThreadContent        `json:"thread,omitempty"`
}

type GmailDraftOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
	Draft           gmailconnector.Draft      `json:"draft,omitempty"`
	Message         gmailconnector.Message    `json:"message,omitempty"`
}

type GmailListDraftsOutput struct {
	Connected          bool                      `json:"connected"`
	AccountRequired    bool                      `json:"account_required,omitempty"`
	Accounts           []integrations.Connection `json:"accounts,omitempty"`
	AccountID          string                    `json:"account_id,omitempty"`
	Alias              string                    `json:"alias,omitempty"`
	Query              string                    `json:"query,omitempty"`
	Drafts             []gmailconnector.Draft    `json:"drafts,omitempty"`
	ResultSizeEstimate int64                     `json:"result_size_estimate,omitempty"`
	NextPageToken      string                    `json:"next_page_token,omitempty"`
}

type GmailThreadContent struct {
	ID        string                `json:"id"`
	HistoryID string                `json:"history_id,omitempty"`
	Snippet   string                `json:"snippet,omitempty"`
	Messages  []GmailMessageContent `json:"messages,omitempty"`
}

type GmailMessageContent struct {
	Message           gmailconnector.Message `json:"message"`
	BodyText          string                 `json:"body_text,omitempty"`
	BodyTextTruncated bool                   `json:"body_text_truncated,omitempty"`
	BodyHTML          string                 `json:"body_html,omitempty"`
	BodyHTMLTruncated bool                   `json:"body_html_truncated,omitempty"`
}
