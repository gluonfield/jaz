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
	MaxMessages int    `json:"max_messages,omitempty" jsonschema:"maximum thread messages to return, 1-20; defaults to 10 most recent messages"`
}

type GmailCreateDraftInput struct {
	Account  string   `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	To       []string `json:"to" jsonschema:"recipient email addresses"`
	Cc       []string `json:"cc,omitempty" jsonschema:"CC recipient email addresses"`
	Bcc      []string `json:"bcc,omitempty" jsonschema:"BCC recipient email addresses"`
	Subject  string   `json:"subject,omitempty" jsonschema:"email subject"`
	BodyText string   `json:"body_text" jsonschema:"plain text email body"`
}

type GmailCreateReplyDraftInput struct {
	Account   string   `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	ID        string   `json:"id" jsonschema:"Gmail message id or thread id to reply to"`
	IDType    string   `json:"id_type,omitempty" jsonschema:"message or thread; defaults to message"`
	ReplyMode string   `json:"reply_mode,omitempty" jsonschema:"reply or reply_all; defaults to reply"`
	BodyText  string   `json:"body_text" jsonschema:"plain text reply body"`
	CcAdd     []string `json:"cc_add,omitempty" jsonschema:"extra CC recipient email addresses to add to the reply draft"`
	BccAdd    []string `json:"bcc_add,omitempty" jsonschema:"BCC recipient email addresses to add to the reply draft"`
}

type GmailSendDraftInput struct {
	Account string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	ID      string `json:"id" jsonschema:"Gmail draft id"`
}

type GmailUpdateDraftInput struct {
	Account  string   `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	ID       string   `json:"id" jsonschema:"Gmail draft id"`
	To       []string `json:"to,omitempty" jsonschema:"replacement recipient email addresses; omit to preserve"`
	Cc       []string `json:"cc,omitempty" jsonschema:"replacement CC recipient email addresses; omit to preserve"`
	Bcc      []string `json:"bcc,omitempty" jsonschema:"replacement BCC recipient email addresses; omit to preserve"`
	Subject  *string  `json:"subject,omitempty" jsonschema:"replacement email subject; omit to preserve"`
	BodyText *string  `json:"body_text,omitempty" jsonschema:"replacement plain text email body; omit to preserve"`
}

type GmailListDraftsInput struct {
	Account    string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	Query      string `json:"query,omitempty" jsonschema:"Gmail draft search query; omit for recent drafts"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum drafts to return, 1-20; defaults to 10"`
}

type GmailReadAttachmentInput struct {
	Account      string `json:"account,omitempty" jsonschema:"Gmail account alias, email address, or connection id; omit only when one Gmail account is connected"`
	MessageID    string `json:"message_id,omitempty" jsonschema:"Gmail message id from gmail_read_thread; can be omitted when attachment_id is an att:gmail/... ref"`
	AttachmentID string `json:"attachment_id" jsonschema:"Gmail attachment id from the message attachments list, or a materialized source ref like att:gmail/account/message/1"`
}

type GmailProfileOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	EmailAddress    string                    `json:"email_address,omitempty"`
	MessagesTotal   int64                     `json:"messages_total,omitempty"`
	ThreadsTotal    int64                     `json:"threads_total,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	AccountName     string                    `json:"account_name,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
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

type GmailReadAttachmentOutput struct {
	Connected            bool                      `json:"connected"`
	AccountRequired      bool                      `json:"account_required,omitempty"`
	Accounts             []integrations.Connection `json:"accounts,omitempty"`
	AccountID            string                    `json:"account_id,omitempty"`
	Alias                string                    `json:"alias,omitempty"`
	MessageID            string                    `json:"message_id,omitempty"`
	AttachmentID         string                    `json:"attachment_id,omitempty"`
	FileName             string                    `json:"file_name,omitempty"`
	MIMEType             string                    `json:"mime_type,omitempty"`
	Size                 int64                     `json:"size,omitempty"`
	FilePath             string                    `json:"file_path,omitempty"`
	TextPreview          string                    `json:"text_preview,omitempty"`
	TextPreviewTruncated bool                      `json:"text_preview_truncated,omitempty"`
	UnsupportedContent   bool                      `json:"unsupported_content,omitempty"`
}

type GmailThreadContent struct {
	ID       string                `json:"id"`
	Snippet  string                `json:"snippet,omitempty"`
	Messages []GmailMessageContent `json:"messages,omitempty"`
}

type GmailMessageContent struct {
	Message           gmailconnector.Message `json:"message"`
	BodyText          string                 `json:"body_text,omitempty"`
	BodyTextTruncated bool                   `json:"body_text_truncated,omitempty"`
}
