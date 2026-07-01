package connections

import (
	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type CalendarGetEventsInput struct {
	Account    string `json:"account,omitempty" jsonschema:"Google Calendar account alias, email address, or connection id; omit only when one Google Calendar account is connected"`
	CalendarID string `json:"calendar_id,omitempty" jsonschema:"calendar id; defaults to primary"`
	TimeMin    string `json:"time_min,omitempty" jsonschema:"inclusive lower bound as RFC3339 date-time, for example 2026-07-02T09:00:00+01:00"`
	TimeMax    string `json:"time_max,omitempty" jsonschema:"exclusive upper bound as RFC3339 date-time, for example 2026-07-02T17:00:00+01:00"`
	Query      string `json:"query,omitempty" jsonschema:"free-text search query for event text fields"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum events to return, 1-50; defaults to 10"`
	PageToken  string `json:"page_token,omitempty" jsonschema:"pagination token from a previous get events result"`
}

type CalendarCreateEventInput struct {
	Account           string   `json:"account,omitempty" jsonschema:"Google Calendar account alias, email address, or connection id; omit only when one Google Calendar account is connected"`
	CalendarID        string   `json:"calendar_id,omitempty" jsonschema:"calendar id; defaults to primary"`
	Summary           string   `json:"summary" jsonschema:"event title"`
	Description       string   `json:"description,omitempty" jsonschema:"event description or agenda"`
	Location          string   `json:"location,omitempty" jsonschema:"event location or meeting link"`
	Start             string   `json:"start" jsonschema:"event start as RFC3339 date-time, or YYYY-MM-DD for all-day events"`
	End               string   `json:"end" jsonschema:"event end as RFC3339 date-time, or exclusive YYYY-MM-DD end date for all-day events"`
	TimeZone          string   `json:"time_zone,omitempty" jsonschema:"IANA time zone such as Europe/London; stored on the Google Calendar event"`
	Attendees         []string `json:"attendees,omitempty" jsonschema:"required attendee email addresses to invite"`
	OptionalAttendees []string `json:"optional_attendees,omitempty" jsonschema:"optional attendee email addresses to invite"`
	SendUpdates       string   `json:"send_updates,omitempty" jsonschema:"all, external_only, or none; defaults to all when attendees are present"`
}

type CalendarEventsOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
	CalendarID      string                    `json:"calendar_id,omitempty"`
	Events          []calendarconnector.Event `json:"events,omitempty"`
	NextPageToken   string                    `json:"next_page_token,omitempty"`
}

type CalendarEventOutput struct {
	Connected       bool                      `json:"connected"`
	AccountRequired bool                      `json:"account_required,omitempty"`
	Accounts        []integrations.Connection `json:"accounts,omitempty"`
	AccountID       string                    `json:"account_id,omitempty"`
	Alias           string                    `json:"alias,omitempty"`
	CalendarID      string                    `json:"calendar_id,omitempty"`
	Event           calendarconnector.Event   `json:"event,omitempty"`
}
