package connections

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func newTestCalendarMCPTools(store CalendarToolStore) *CalendarMCPTools {
	return NewCalendarMCPTools(store)
}

func TestCalendarMCPToolsGetEventsAndCreateEvent(t *testing.T) {
	calendarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.EscapedPath() {
		case "/calendar/v3/calendars/primary/events":
			switch r.Method {
			case http.MethodGet:
				q := r.URL.Query()
				if q.Get("timeMin") != "2026-07-02T09:00:00+01:00" ||
					q.Get("timeMax") != "2026-07-02T17:00:00+01:00" ||
					q.Get("q") != "planning" ||
					q.Get("maxResults") != "5" ||
					q.Get("singleEvents") != "true" ||
					q.Get("orderBy") != "startTime" {
					t.Fatalf("query = %#v", q)
				}
				_, _ = w.Write([]byte(`{
					"items":[{
						"id":"evt1",
						"summary":"Planning",
						"start":{"dateTime":"2026-07-02T10:00:00+01:00","timeZone":"Europe/London"},
						"end":{"dateTime":"2026-07-02T10:30:00+01:00","timeZone":"Europe/London"}
					}]
				}`))
			case http.MethodPost:
				if r.URL.Query().Get("sendUpdates") != "all" {
					t.Fatalf("query = %#v", r.URL.Query())
				}
				var body struct {
					Summary     string `json:"summary"`
					Description string `json:"description"`
					Location    string `json:"location"`
					Start       struct {
						DateTime string `json:"dateTime"`
						TimeZone string `json:"timeZone"`
					} `json:"start"`
					End struct {
						DateTime string `json:"dateTime"`
						TimeZone string `json:"timeZone"`
					} `json:"end"`
					Attendees []struct {
						Email       string `json:"email"`
						DisplayName string `json:"displayName"`
						Optional    bool   `json:"optional"`
					} `json:"attendees"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Summary != "Demo" ||
					body.Description != "Agenda" ||
					body.Location != "Oxford" ||
					body.Start.DateTime != "2026-07-02T14:00:00+01:00" ||
					body.End.DateTime != "2026-07-02T14:30:00+01:00" ||
					body.Start.TimeZone != "Europe/London" ||
					len(body.Attendees) != 2 ||
					body.Attendees[0].Email != "alice@example.com" ||
					body.Attendees[0].DisplayName != "Alice" ||
					body.Attendees[1].Email != "bob@example.com" ||
					body.Attendees[1].Optional != true {
					t.Fatalf("body = %#v", body)
				}
				_, _ = w.Write([]byte(`{
					"id":"evt2",
					"summary":"Demo",
					"htmlLink":"https://calendar.google.com/event?eid=evt2",
					"start":{"dateTime":"2026-07-02T14:00:00+01:00","timeZone":"Europe/London"},
					"end":{"dateTime":"2026-07-02T14:30:00+01:00","timeZone":"Europe/London"},
					"attendees":[{"email":"alice@example.com"},{"email":"bob@example.com","optional":true}]
				}`))
			default:
				t.Fatalf("method = %s", r.Method)
			}
		default:
			t.Fatalf("path = %s", r.URL.EscapedPath())
		}
	}))
	defer calendarServer.Close()

	tools := newTestCalendarMCPTools(&testConnectionStore{
		tokens: map[string]integrationoauth.Token{
			"google_calendar:personal": {
				AccessToken: "access",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(time.Hour),
			},
		},
		connections: []integrations.Connection{{
			ID:        "google_calendar:personal",
			Provider:  calendarconnector.ProviderID,
			AccountID: "augustinas@example.com",
			Alias:     "personal",
		}},
	})
	tools.apiBaseURL = calendarServer.URL

	result, events, err := tools.GetEvents(context.Background(), nil, CalendarGetEventsInput{
		TimeMin:    "2026-07-02T09:00:00+01:00",
		TimeMax:    "2026-07-02T17:00:00+01:00",
		Query:      " planning ",
		MaxResults: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !events.Connected || events.CalendarID != "primary" || len(events.Events) != 1 || events.Events[0].ID != "evt1" {
		t.Fatalf("events = %#v", events)
	}
	if got := toolText(result); !strings.Contains(got, "Found 1 Google Calendar events") {
		t.Fatalf("text = %q", got)
	}

	result, created, err := tools.CreateEvent(context.Background(), nil, CalendarCreateEventInput{
		Summary:           " Demo ",
		Description:       " Agenda ",
		Location:          " Oxford ",
		Start:             "2026-07-02T14:00:00+01:00",
		End:               "2026-07-02T14:30:00+01:00",
		TimeZone:          " Europe/London ",
		Attendees:         []string{" Alice <alice@example.com> "},
		OptionalAttendees: []string{"alice@example.com", "bob@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Connected || created.Event.ID != "evt2" || created.CalendarID != "primary" || len(created.Event.Attendees) != 2 {
		t.Fatalf("created = %#v", created)
	}
	if got := toolText(result); !strings.Contains(got, "Created Google Calendar event: Demo") {
		t.Fatalf("text = %q", got)
	}
}

func TestCalendarMCPToolsReportsNotConnected(t *testing.T) {
	_, out, err := newTestCalendarMCPTools(&testConnectionStore{}).GetEvents(context.Background(), nil, CalendarGetEventsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Connected {
		t.Fatalf("out = %#v", out)
	}
}

func TestCalendarMCPToolsRequireAccountWhenMultipleAccountsConnected(t *testing.T) {
	tools := newTestCalendarMCPTools(&testConnectionStore{connections: []integrations.Connection{{
		ID:        "google_calendar:personal",
		Provider:  calendarconnector.ProviderID,
		AccountID: "augustinas@example.com",
		Alias:     "personal",
	}, {
		ID:        "google_calendar:work",
		Provider:  calendarconnector.ProviderID,
		AccountID: "augustinas@work.example",
		Alias:     "work",
	}}})

	result, out, err := tools.GetEvents(context.Background(), nil, CalendarGetEventsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.AccountRequired || !out.Connected {
		t.Fatalf("out = %#v", out)
	}
	if got := toolText(result); !strings.Contains(got, "Specify account") || !strings.Contains(got, "personal") || !strings.Contains(got, "work") {
		t.Fatalf("text = %q", got)
	}
}

func TestCalendarMCPToolsValidateCreateEventInput(t *testing.T) {
	_, _, err := newTestCalendarMCPTools(&testConnectionStore{}).CreateEvent(context.Background(), nil, CalendarCreateEventInput{
		Summary:     "Demo",
		Start:       "tomorrow",
		End:         "2026-07-02T14:30:00+01:00",
		SendUpdates: "loudly",
	})
	if err == nil || !strings.Contains(err.Error(), "start must be") {
		t.Fatalf("err = %v", err)
	}
}
