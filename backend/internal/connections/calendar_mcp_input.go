package connections

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
)

func calendarGetEventsRequest(input CalendarGetEventsInput) (calendarconnector.GetEventsRequest, error) {
	timeMin, err := calendarOptionalTime(input.TimeMin, "time_min")
	if err != nil {
		return calendarconnector.GetEventsRequest{}, err
	}
	timeMax, err := calendarOptionalTime(input.TimeMax, "time_max")
	if err != nil {
		return calendarconnector.GetEventsRequest{}, err
	}
	return calendarconnector.GetEventsRequest{
		CalendarID: strings.TrimSpace(input.CalendarID),
		TimeMin:    timeMin,
		TimeMax:    timeMax,
		Query:      strings.TrimSpace(input.Query),
		MaxResults: calendarEventLimit(input.MaxResults),
		PageToken:  strings.TrimSpace(input.PageToken),
	}, nil
}

func calendarCreateEventRequest(input CalendarCreateEventInput) (calendarconnector.CreateEventRequest, error) {
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		return calendarconnector.CreateEventRequest{}, errors.New("summary is required")
	}
	start, err := calendarEventTime(input.Start, input.TimeZone, "start")
	if err != nil {
		return calendarconnector.CreateEventRequest{}, err
	}
	end, err := calendarEventTime(input.End, input.TimeZone, "end")
	if err != nil {
		return calendarconnector.CreateEventRequest{}, err
	}
	seenAttendees := map[string]struct{}{}
	attendees, err := calendarAttendees(input.Attendees, false, seenAttendees)
	if err != nil {
		return calendarconnector.CreateEventRequest{}, err
	}
	optional, err := calendarAttendees(input.OptionalAttendees, true, seenAttendees)
	if err != nil {
		return calendarconnector.CreateEventRequest{}, err
	}
	attendees = append(attendees, optional...)
	sendUpdates, err := calendarconnector.NormalizeSendUpdates(input.SendUpdates, len(attendees))
	if err != nil {
		return calendarconnector.CreateEventRequest{}, err
	}
	return calendarconnector.CreateEventRequest{
		CalendarID:  strings.TrimSpace(input.CalendarID),
		Summary:     summary,
		Description: strings.TrimSpace(input.Description),
		Location:    strings.TrimSpace(input.Location),
		Start:       start,
		End:         end,
		Attendees:   attendees,
		SendUpdates: sendUpdates,
	}, nil
}

func calendarOptionalTime(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.New(field + " must be an RFC3339 date-time")
	}
	return parsed, nil
}

func calendarEventTime(value, timeZone, field string) (calendarconnector.EventTimeInput, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return calendarconnector.EventTimeInput{}, errors.New(field + " is required")
	}
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return calendarconnector.EventTimeInput{Date: value, TimeZone: strings.TrimSpace(timeZone)}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return calendarconnector.EventTimeInput{}, errors.New(field + " must be an RFC3339 date-time or YYYY-MM-DD date")
	}
	return calendarconnector.EventTimeInput{DateTime: parsed, TimeZone: strings.TrimSpace(timeZone)}, nil
}

func calendarAttendees(values []string, optional bool, seen map[string]struct{}) ([]calendarconnector.AttendeeInput, error) {
	out := make([]calendarconnector.AttendeeInput, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		address, err := mail.ParseAddress(value)
		if err != nil {
			return nil, err
		}
		email := strings.ToLower(strings.TrimSpace(address.Address))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, calendarconnector.AttendeeInput{
			Email:       address.Address,
			DisplayName: strings.TrimSpace(address.Name),
			Optional:    optional,
		})
	}
	return out, nil
}
