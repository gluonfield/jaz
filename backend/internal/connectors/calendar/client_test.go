package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIClientUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/v2/userinfo" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"email":"augustinas@example.com","name":"Augustinas"}`))
	}))
	defer server.Close()

	info, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).UserInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Email != "augustinas@example.com" || info.Name != "Augustinas" {
		t.Fatalf("info = %#v", info)
	}
}

func TestAPIClientGetEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/calendar/v3/calendars/team@example.com/events" {
			t.Fatalf("path = %s", r.URL.EscapedPath())
		}
		q := r.URL.Query()
		if q.Get("timeMin") != "2026-07-02T09:00:00Z" ||
			q.Get("timeMax") != "2026-07-02T17:00:00Z" ||
			q.Get("q") != "planning" ||
			q.Get("maxResults") != "5" ||
			q.Get("pageToken") != "next" ||
			q.Get("singleEvents") != "true" ||
			q.Get("orderBy") != "startTime" {
			t.Fatalf("query = %#v", q)
		}
		_, _ = w.Write([]byte(`{
			"nextPageToken":"later",
			"items":[{
				"id":"evt1",
				"status":"confirmed",
				"summary":"Planning",
				"description":"Roadmap",
				"location":"Office",
				"htmlLink":"https://calendar.google.com/event?eid=evt1",
				"start":{"dateTime":"2026-07-02T10:00:00+01:00","timeZone":"Europe/London"},
				"end":{"dateTime":"2026-07-02T10:30:00+01:00","timeZone":"Europe/London"},
				"attendees":[{"email":"alice@example.com","displayName":"Alice","responseStatus":"accepted"}],
				"creator":{"email":"augustinas@example.com","self":true},
				"organizer":{"email":"team@example.com"},
				"created":"2026-07-01T12:00:00Z",
				"updated":"2026-07-01T12:01:00Z"
			}]
		}`))
	}))
	defer server.Close()

	timeMin := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	timeMax := time.Date(2026, 7, 2, 17, 0, 0, 0, time.UTC)
	events, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).GetEvents(context.Background(), GetEventsRequest{
		CalendarID: "team@example.com",
		TimeMin:    timeMin,
		TimeMax:    timeMax,
		Query:      "planning",
		MaxResults: 5,
		PageToken:  "next",
	})
	if err != nil {
		t.Fatal(err)
	}
	if events.CalendarID != "team@example.com" || events.NextPageToken != "later" || len(events.Events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	event := events.Events[0]
	if event.ID != "evt1" || event.Summary != "Planning" || event.Start.DateTime != "2026-07-02T10:00:00+01:00" || len(event.Attendees) != 1 || event.Creator.Email != "augustinas@example.com" {
		t.Fatalf("event = %#v", event)
	}
}

func TestAPIClientCreateEventInvitesGuests(t *testing.T) {
	start := time.Date(2026, 7, 2, 14, 0, 0, 0, time.FixedZone("BST", 3600))
	end := start.Add(30 * time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/calendar/v3/calendars/primary/events" || r.Method != http.MethodPost {
			t.Fatalf("%s %s", r.Method, r.URL.EscapedPath())
		}
		if r.URL.Query().Get("sendUpdates") != "all" {
			t.Fatalf("query = %#v", r.URL.Query())
		}
		var body apiEvent
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Summary != "Demo" ||
			body.Description != "Show Jaz" ||
			body.Location != "Oxford" ||
			body.Start.DateTime != "2026-07-02T14:00:00+01:00" ||
			body.End.DateTime != "2026-07-02T14:30:00+01:00" ||
			body.Start.TimeZone != "Europe/London" ||
			len(body.Attendees) != 2 ||
			body.Attendees[0].Email != "alice@example.com" ||
			body.Attendees[1].Optional != true {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{
			"id":"evt1",
			"summary":"Demo",
			"start":{"dateTime":"2026-07-02T14:00:00+01:00","timeZone":"Europe/London"},
			"end":{"dateTime":"2026-07-02T14:30:00+01:00","timeZone":"Europe/London"},
			"attendees":[{"email":"alice@example.com"},{"email":"bob@example.com","optional":true}]
		}`))
	}))
	defer server.Close()

	event, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).CreateEvent(context.Background(), CreateEventRequest{
		Summary:     " Demo ",
		Description: " Show Jaz ",
		Location:    " Oxford ",
		Start:       EventTimeInput{DateTime: start, TimeZone: " Europe/London "},
		End:         EventTimeInput{DateTime: end, TimeZone: " Europe/London "},
		Attendees: []AttendeeInput{
			{Email: " alice@example.com "},
			{Email: "bob@example.com", Optional: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "evt1" || event.Summary != "Demo" || len(event.Attendees) != 2 {
		t.Fatalf("event = %#v", event)
	}
}

func TestAPIClientCreateEventRejectsInvalidDate(t *testing.T) {
	_, err := (APIClient{}).CreateEvent(context.Background(), CreateEventRequest{
		Summary: "Demo",
		Start:   EventTimeInput{Date: "tomorrow"},
		End:     EventTimeInput{Date: "2026-07-03"},
	})
	if err == nil || !strings.Contains(err.Error(), "start date") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPIClientCreateEventRejectsInvalidSendUpdates(t *testing.T) {
	_, err := (APIClient{}).CreateEvent(context.Background(), CreateEventRequest{
		Summary:     "Demo",
		Start:       EventTimeInput{Date: "2026-07-02"},
		End:         EventTimeInput{Date: "2026-07-03"},
		SendUpdates: "loudly",
	})
	if err == nil || !strings.Contains(err.Error(), "send_updates") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPIClientNormalizesDisabledAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Calendar API has not been used in project 123456789 before or it is disabled. Enable it by visiting https://console.developers.google.com/apis/api/calendar-json.googleapis.com/overview?project=123456789 then retry.",
				"errors": [{"reason":"accessNotConfigured"}],
				"details": [{"reason":"SERVICE_DISABLED"}]
			}
		}`))
	}))
	defer server.Close()

	_, err := (APIClient{HTTP: server.Client(), BaseURL: server.URL}).UserInfo(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "google calendar api is disabled for the OAuth client project") || strings.Contains(text, "123456789") {
		t.Fatalf("error = %q", text)
	}
}
