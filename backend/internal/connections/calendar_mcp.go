package connections

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
)

type CalendarMCPTools struct {
	store      CalendarToolStore
	apiBaseURL string
}

func NewCalendarMCPTools(store CalendarToolStore) *CalendarMCPTools {
	return &CalendarMCPTools{store: store}
}

func (t *CalendarMCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        calendarconnector.ToolGetEvents,
		Title:       "Get Google Calendar events",
		Description: "Get Google Calendar events from one connected account. If multiple Google Calendar accounts are connected, pass account as an alias, email, or connection id.",
	}, t.GetEvents)
	mcp.AddTool(server, &mcp.Tool{
		Name:        calendarconnector.ToolCreateEvent,
		Title:       "Create Google Calendar event",
		Description: "Create a Google Calendar event on one connected account, with optional location, description, attendees, optional attendees, and guest update emails.",
	}, t.CreateEvent)
}

func (t *CalendarMCPTools) RemoveFrom(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(calendarconnector.ToolGetEvents, calendarconnector.ToolCreateEvent)
	}
}

func (t *CalendarMCPTools) GetEvents(ctx context.Context, _ *mcp.CallToolRequest, input CalendarGetEventsInput) (*mcp.CallToolResult, CalendarEventsOutput, error) {
	request, err := calendarGetEventsRequest(input)
	if err != nil {
		return nil, CalendarEventsOutput{}, err
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, CalendarEventsOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := CalendarEventsOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(mcpAccountRequiredText(calendarconnector.ProviderName, session.accounts)), out, nil
		}
		return textResult("Google Calendar is not connected. Connect Google Calendar in Settings > Connections."), out, nil
	}
	events, err := session.api.GetEvents(ctx, request)
	if err != nil {
		return nil, CalendarEventsOutput{}, err
	}
	out := CalendarEventsOutput{
		Connected:     true,
		Accounts:      session.accounts,
		AccountID:     session.connection.AccountID,
		Alias:         session.connection.Alias,
		CalendarID:    events.CalendarID,
		Events:        events.Events,
		NextPageToken: events.NextPageToken,
	}
	return textResult(fmt.Sprintf("Found %d Google Calendar events.", len(events.Events))), out, nil
}

func (t *CalendarMCPTools) CreateEvent(ctx context.Context, _ *mcp.CallToolRequest, input CalendarCreateEventInput) (*mcp.CallToolResult, CalendarEventOutput, error) {
	request, err := calendarCreateEventRequest(input)
	if err != nil {
		return nil, CalendarEventOutput{}, err
	}
	session, connected, err := t.session(ctx, input.Account)
	if err != nil {
		return nil, CalendarEventOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := CalendarEventOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(mcpAccountRequiredText(calendarconnector.ProviderName, session.accounts)), out, nil
		}
		return textResult("Google Calendar is not connected. Connect Google Calendar in Settings > Connections."), out, nil
	}
	event, err := session.api.CreateEvent(ctx, request)
	if err != nil {
		return nil, CalendarEventOutput{}, err
	}
	text := "Created Google Calendar event"
	if event.Summary != "" {
		text += ": " + event.Summary
	}
	return textResult(text), CalendarEventOutput{
		Connected:  true,
		Accounts:   session.accounts,
		AccountID:  session.connection.AccountID,
		Alias:      session.connection.Alias,
		CalendarID: event.CalendarID,
		Event:      event,
	}, nil
}
