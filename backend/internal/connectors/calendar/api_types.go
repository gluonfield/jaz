package calendar

import (
	"strings"
)

type eventList struct {
	Items         []apiEvent `json:"items"`
	NextPageToken string     `json:"nextPageToken"`
}

type apiEvent struct {
	ID          string         `json:"id,omitempty"`
	Status      string         `json:"status,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	Location    string         `json:"location,omitempty"`
	HTMLLink    string         `json:"htmlLink,omitempty"`
	Start       apiEventTime   `json:"start,omitempty"`
	End         apiEventTime   `json:"end,omitempty"`
	Attendees   []apiAttendee  `json:"attendees,omitempty"`
	Creator     apiEventPerson `json:"creator,omitempty"`
	Organizer   apiEventPerson `json:"organizer,omitempty"`
	Created     string         `json:"created,omitempty"`
	Updated     string         `json:"updated,omitempty"`
}

type apiEventTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type apiAttendee struct {
	Email          string `json:"email,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
}

type apiEventPerson struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Self        bool   `json:"self,omitempty"`
}

func eventFromAPI(calendarID string, raw apiEvent) Event {
	attendees := make([]Attendee, 0, len(raw.Attendees))
	for _, attendee := range raw.Attendees {
		if attendee.Email == "" {
			continue
		}
		attendees = append(attendees, Attendee{
			Email:          attendee.Email,
			DisplayName:    attendee.DisplayName,
			Optional:       attendee.Optional,
			ResponseStatus: attendee.ResponseStatus,
		})
	}
	return Event{
		ID:          raw.ID,
		CalendarID:  calendarID,
		Status:      raw.Status,
		Summary:     raw.Summary,
		Description: raw.Description,
		Location:    raw.Location,
		HTMLLink:    raw.HTMLLink,
		Start:       eventTimeFromAPI(raw.Start),
		End:         eventTimeFromAPI(raw.End),
		Attendees:   attendees,
		Creator:     eventPersonFromAPI(raw.Creator),
		Organizer:   eventPersonFromAPI(raw.Organizer),
		Created:     strings.TrimSpace(raw.Created),
		Updated:     strings.TrimSpace(raw.Updated),
	}
}

func eventTimeFromAPI(raw apiEventTime) EventTime {
	return EventTime{
		DateTime: strings.TrimSpace(raw.DateTime),
		Date:     strings.TrimSpace(raw.Date),
		TimeZone: strings.TrimSpace(raw.TimeZone),
	}
}

func eventPersonFromAPI(raw apiEventPerson) EventPerson {
	return EventPerson{
		Email:       raw.Email,
		DisplayName: raw.DisplayName,
		Self:        raw.Self,
	}
}
