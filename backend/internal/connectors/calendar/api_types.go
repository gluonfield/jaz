package calendar

import (
	"strings"
)

type eventList struct {
	Items         []apiEvent `json:"items"`
	NextPageToken string     `json:"nextPageToken"`
}

type apiEvent struct {
	ID             string             `json:"id,omitempty"`
	Status         string             `json:"status,omitempty"`
	Summary        string             `json:"summary,omitempty"`
	Description    string             `json:"description,omitempty"`
	Location       string             `json:"location,omitempty"`
	HTMLLink       string             `json:"htmlLink,omitempty"`
	HangoutLink    string             `json:"hangoutLink,omitempty"`
	ConferenceData *apiConferenceData `json:"conferenceData,omitempty"`
	Start          apiEventTime       `json:"start,omitempty"`
	End            apiEventTime       `json:"end,omitempty"`
	Attendees      []apiAttendee      `json:"attendees,omitempty"`
	Creator        apiEventPerson     `json:"creator,omitempty"`
	Organizer      apiEventPerson     `json:"organizer,omitempty"`
	Created        string             `json:"created,omitempty"`
	Updated        string             `json:"updated,omitempty"`
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

type apiConferenceData struct {
	CreateRequest      *apiConferenceCreateRequest `json:"createRequest,omitempty"`
	EntryPoints        []apiConferenceEntryPoint   `json:"entryPoints,omitempty"`
	ConferenceSolution *apiConferenceSolution      `json:"conferenceSolution,omitempty"`
	ConferenceID       string                      `json:"conferenceId,omitempty"`
	Notes              string                      `json:"notes,omitempty"`
}

type apiConferenceCreateRequest struct {
	RequestID             string                     `json:"requestId,omitempty"`
	ConferenceSolutionKey apiConferenceSolutionKey   `json:"conferenceSolutionKey"`
	Status                *apiConferenceCreateStatus `json:"status,omitempty"`
}

type apiConferenceCreateStatus struct {
	StatusCode string `json:"statusCode,omitempty"`
}

type apiConferenceSolution struct {
	Key     apiConferenceSolutionKey `json:"key"`
	Name    string                   `json:"name,omitempty"`
	IconURI string                   `json:"iconUri,omitempty"`
}

type apiConferenceSolutionKey struct {
	Type string `json:"type"`
}

type apiConferenceEntryPoint struct {
	Type        string `json:"entryPointType,omitempty"`
	URI         string `json:"uri,omitempty"`
	Label       string `json:"label,omitempty"`
	PIN         string `json:"pin,omitempty"`
	AccessCode  string `json:"accessCode,omitempty"`
	MeetingCode string `json:"meetingCode,omitempty"`
	Passcode    string `json:"passcode,omitempty"`
	Password    string `json:"password,omitempty"`
}

func eventFromAPI(calendarID string, raw apiEvent) Event {
	attendees := make([]Attendee, 0, len(raw.Attendees))
	for _, attendee := range raw.Attendees {
		if attendee.Email == "" {
			continue
		}
		attendees = append(attendees, Attendee(attendee))
	}
	return Event{
		ID:             raw.ID,
		CalendarID:     calendarID,
		Status:         raw.Status,
		Summary:        raw.Summary,
		Description:    raw.Description,
		Location:       raw.Location,
		HTMLLink:       raw.HTMLLink,
		HangoutLink:    raw.HangoutLink,
		ConferenceData: conferenceDataFromAPI(raw.ConferenceData),
		Start:          eventTimeFromAPI(raw.Start),
		End:            eventTimeFromAPI(raw.End),
		Attendees:      attendees,
		Creator:        EventPerson(raw.Creator),
		Organizer:      EventPerson(raw.Organizer),
		Created:        strings.TrimSpace(raw.Created),
		Updated:        strings.TrimSpace(raw.Updated),
	}
}

func conferenceDataFromAPI(raw *apiConferenceData) *ConferenceData {
	if raw == nil {
		return nil
	}
	out := &ConferenceData{
		ConferenceID: raw.ConferenceID,
		EntryPoints:  make([]ConferenceEntryPoint, 0, len(raw.EntryPoints)),
		Notes:        raw.Notes,
	}
	if raw.CreateRequest != nil && raw.CreateRequest.Status != nil {
		out.CreateRequestStatus = raw.CreateRequest.Status.StatusCode
	}
	if raw.ConferenceSolution != nil {
		out.Solution = &ConferenceSolution{
			Type:    raw.ConferenceSolution.Key.Type,
			Name:    raw.ConferenceSolution.Name,
			IconURI: raw.ConferenceSolution.IconURI,
		}
	}
	for _, entryPoint := range raw.EntryPoints {
		out.EntryPoints = append(out.EntryPoints, ConferenceEntryPoint(entryPoint))
	}
	return out
}

func eventTimeFromAPI(raw apiEventTime) EventTime {
	return EventTime{
		DateTime: strings.TrimSpace(raw.DateTime),
		Date:     strings.TrimSpace(raw.Date),
		TimeZone: strings.TrimSpace(raw.TimeZone),
	}
}
