package connections

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
)

type GmailMCPTools struct {
	store      GmailToolStore
	apiBaseURL string
}

func NewGmailMCPTools(store GmailToolStore) *GmailMCPTools {
	return &GmailMCPTools{store: store}
}

func (t *GmailMCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolGetProfile,
		Title:       "Get Gmail profile",
		Description: "Show live profile totals for one connected Gmail account. If multiple Gmail accounts are connected, pass account as an alias, email, or connection id.",
	}, t.GetProfile)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolSearchThreads,
		Title:       "Search Gmail threads",
		Description: "Search or list Gmail conversation threads for one connected account and return thread IDs with summarized message metadata. If multiple Gmail accounts are connected, pass account as an alias, email, or connection id.",
	}, t.SearchThreads)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolReadThread,
		Title:       "Read Gmail thread",
		Description: "Read a full Gmail conversation thread by message ID or thread ID before drafting a reply.",
	}, t.ReadThread)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolCreateDraft,
		Title:       "Create Gmail draft",
		Description: "Create a new plain text Gmail draft from one connected account. Use this for new outbound email.",
	}, t.CreateDraft)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolCreateReply,
		Title:       "Create Gmail reply draft",
		Description: "Create a reply or reply-all draft for an existing Gmail message or thread. Use this for normal replies; it infers recipients, thread_id, In-Reply-To, References, and reply subject.",
	}, t.CreateReplyDraft)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolSendDraft,
		Title:       "Send Gmail draft",
		Description: "Send an existing Gmail draft after review or explicit approval.",
	}, t.SendDraft)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolUpdateDraft,
		Title:       "Update Gmail draft",
		Description: "Update an existing Gmail draft while preserving omitted fields.",
	}, t.UpdateDraft)
	mcp.AddTool(server, &mcp.Tool{
		Name:        gmailconnector.ToolListDrafts,
		Title:       "List Gmail drafts",
		Description: "List Gmail drafts with summarized metadata so a draft can be reviewed or selected.",
	}, t.ListDrafts)
}

func (t *GmailMCPTools) RemoveFrom(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(
			gmailconnector.ToolGetProfile,
			gmailconnector.ToolSearchThreads,
			gmailconnector.ToolReadThread,
			gmailconnector.ToolCreateDraft,
			gmailconnector.ToolCreateReply,
			gmailconnector.ToolSendDraft,
			gmailconnector.ToolUpdateDraft,
			gmailconnector.ToolListDrafts,
		)
	}
}

func (t *GmailMCPTools) GetProfile(ctx context.Context, _ *mcp.CallToolRequest, input GmailProfileInput) (*mcp.CallToolResult, GmailProfileOutput, error) {
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailProfileOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailProfileOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	profile, err := session.api.Profile(ctx)
	if err != nil {
		return nil, GmailProfileOutput{}, err
	}
	out := GmailProfileOutput{
		Connected:     true,
		Accounts:      session.accounts,
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

func (t *GmailMCPTools) SearchThreads(ctx context.Context, _ *mcp.CallToolRequest, input GmailSearchThreadsInput) (*mcp.CallToolResult, GmailSearchThreadsOutput, error) {
	query := strings.TrimSpace(input.Query)
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailSearchThreadsOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailSearchThreadsOutput{Connected: accountRequired, Query: query, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	search, err := session.api.SearchThreads(ctx, gmailconnector.SearchThreadsRequest{
		Query:      query,
		MaxResults: gmailSearchLimit(input.MaxResults),
	})
	if err != nil {
		return nil, GmailSearchThreadsOutput{}, err
	}
	out := GmailSearchThreadsOutput{
		Connected:          true,
		Accounts:           session.accounts,
		AccountID:          session.connection.AccountID,
		Alias:              session.connection.Alias,
		Query:              query,
		Threads:            search.Threads,
		ResultSizeEstimate: search.ResultSizeEstimate,
		NextPageToken:      search.NextPageToken,
	}
	text := fmt.Sprintf("Found %d Gmail threads.", len(search.Threads))
	if query != "" {
		text = fmt.Sprintf("Found %d Gmail threads for %q.", len(search.Threads), query)
	}
	return textResult(text), out, nil
}

func (t *GmailMCPTools) ReadThread(ctx context.Context, _ *mcp.CallToolRequest, input GmailReadThreadInput) (*mcp.CallToolResult, GmailReadThreadOutput, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, GmailReadThreadOutput{}, errors.New("id is required")
	}
	idType := gmailThreadIDType(input.IDType)
	if idType == "" {
		return nil, GmailReadThreadOutput{}, errors.New("id_type must be message or thread")
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailReadThreadOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailReadThreadOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	content, err := session.api.ReadThread(ctx, gmailconnector.ReadThreadRequest{
		ID:          id,
		IDType:      idType,
		MaxMessages: gmailThreadLimit(input.MaxMessages),
	})
	if err != nil {
		return nil, GmailReadThreadOutput{}, err
	}
	subject := ""
	if len(content.Messages) > 0 {
		subject = content.Messages[0].Message.Subject
	}
	if subject == "" {
		subject = id
	}
	return textResult("Read Gmail thread: " + subject), GmailReadThreadOutput{
		Connected: true,
		Accounts:  session.accounts,
		AccountID: session.connection.AccountID,
		Alias:     session.connection.Alias,
		Thread:    gmailToolThread(content),
	}, nil
}

func (t *GmailMCPTools) CreateDraft(ctx context.Context, _ *mcp.CallToolRequest, input GmailCreateDraftInput) (*mcp.CallToolResult, GmailDraftOutput, error) {
	request, err := gmailCreateDraftRequest(input)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailDraftOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	draft, err := session.api.CreateDraft(ctx, request)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	text := "Created Gmail draft"
	if request.Subject != "" {
		text += ": " + request.Subject
	}
	return textResult(text), GmailDraftOutput{
		Connected: true,
		Accounts:  session.accounts,
		AccountID: session.connection.AccountID,
		Alias:     session.connection.Alias,
		Draft:     draft,
	}, nil
}

func (t *GmailMCPTools) CreateReplyDraft(ctx context.Context, _ *mcp.CallToolRequest, input GmailCreateReplyDraftInput) (*mcp.CallToolResult, GmailDraftOutput, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, GmailDraftOutput{}, errors.New("id is required")
	}
	idType := gmailThreadIDType(input.IDType)
	if idType == "" {
		return nil, GmailDraftOutput{}, errors.New("id_type must be message or thread")
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailDraftOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	thread, err := session.api.ReadThread(ctx, gmailconnector.ReadThreadRequest{
		ID:          id,
		IDType:      idType,
		MaxMessages: gmailThreadLimit(0),
	})
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	request, err := gmailReplyDraftRequest(input, thread, session.connection.AccountID)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	draft, err := session.api.CreateDraft(ctx, request)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	text := "Created Gmail reply draft"
	if request.Subject != "" {
		text += ": " + request.Subject
	}
	return textResult(text), GmailDraftOutput{
		Connected: true,
		Accounts:  session.accounts,
		AccountID: session.connection.AccountID,
		Alias:     session.connection.Alias,
		Draft:     draft,
	}, nil
}

func (t *GmailMCPTools) SendDraft(ctx context.Context, _ *mcp.CallToolRequest, input GmailSendDraftInput) (*mcp.CallToolResult, GmailDraftOutput, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, GmailDraftOutput{}, errors.New("id is required")
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailDraftOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	message, err := session.api.SendDraft(ctx, id)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	text := "Sent Gmail draft"
	if message.Subject != "" {
		text += ": " + message.Subject
	}
	return textResult(text), GmailDraftOutput{
		Connected: true,
		Accounts:  session.accounts,
		AccountID: session.connection.AccountID,
		Alias:     session.connection.Alias,
		Message:   message,
	}, nil
}

func (t *GmailMCPTools) UpdateDraft(ctx context.Context, _ *mcp.CallToolRequest, input GmailUpdateDraftInput) (*mcp.CallToolResult, GmailDraftOutput, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, GmailDraftOutput{}, errors.New("id is required")
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailDraftOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	current, err := session.api.GetDraft(ctx, id)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	request, err := gmailUpdateDraftRequest(input, current)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	draft, err := session.api.UpdateDraft(ctx, id, request)
	if err != nil {
		return nil, GmailDraftOutput{}, err
	}
	text := "Updated Gmail draft"
	if draft.Message.Subject != "" {
		text += ": " + draft.Message.Subject
	}
	return textResult(text), GmailDraftOutput{
		Connected: true,
		Accounts:  session.accounts,
		AccountID: session.connection.AccountID,
		Alias:     session.connection.Alias,
		Draft:     draft,
	}, nil
}

func (t *GmailMCPTools) ListDrafts(ctx context.Context, _ *mcp.CallToolRequest, input GmailListDraftsInput) (*mcp.CallToolResult, GmailListDraftsOutput, error) {
	query := strings.TrimSpace(input.Query)
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, GmailListDraftsOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailListDraftsOutput{Connected: accountRequired, Query: query, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	drafts, err := session.api.ListDrafts(ctx, gmailconnector.ListDraftsRequest{
		Query:      query,
		MaxResults: gmailSearchLimit(input.MaxResults),
	})
	if err != nil {
		return nil, GmailListDraftsOutput{}, err
	}
	out := GmailListDraftsOutput{
		Connected:          true,
		Accounts:           session.accounts,
		AccountID:          session.connection.AccountID,
		Alias:              session.connection.Alias,
		Query:              query,
		Drafts:             drafts.Drafts,
		ResultSizeEstimate: drafts.ResultSizeEstimate,
		NextPageToken:      drafts.NextPageToken,
	}
	text := fmt.Sprintf("Found %d Gmail drafts.", len(drafts.Drafts))
	if query != "" {
		text = fmt.Sprintf("Found %d Gmail drafts for %q.", len(drafts.Drafts), query)
	}
	return textResult(text), out, nil
}
