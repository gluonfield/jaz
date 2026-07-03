package calendar

import "time"

const CalendarIDPrimary = "primary"

type UserInfo struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type GetEventsRequest struct {
	CalendarID string
	TimeMin    time.Time
	TimeMax    time.Time
	Query      string
	MaxResults int
	PageToken  string
}

type GetEventsResponse struct {
	CalendarID    string  `json:"calendar_id"`
	Events        []Event `json:"events,omitempty"`
	NextPageToken string  `json:"next_page_token,omitempty"`
}

type CreateEventRequest struct {
	CalendarID  string
	Summary     string
	Description string
	Location    string
	Start       EventTimeInput
	End         EventTimeInput
	Attendees   []AttendeeInput
	SendUpdates string
}

type EventTimeInput struct {
	DateTime time.Time
	Date     string
	TimeZone string
}

type AttendeeInput struct {
	Email       string
	DisplayName string
	Optional    bool
}

type Event struct {
	ID          string      `json:"id"`
	CalendarID  string      `json:"calendar_id,omitempty"`
	Status      string      `json:"status,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	Description string      `json:"description,omitempty"`
	Location    string      `json:"location,omitempty"`
	HTMLLink    string      `json:"html_link,omitempty"`
	Start       EventTime   `json:"start,omitempty"`
	End         EventTime   `json:"end,omitempty"`
	Attendees   []Attendee  `json:"attendees,omitempty"`
	Creator     EventPerson `json:"creator,omitempty"`
	Organizer   EventPerson `json:"organizer,omitempty"`
	Created     string      `json:"created,omitempty"`
	Updated     string      `json:"updated,omitempty"`
}

type EventTime struct {
	DateTime string `json:"date_time,omitempty"`
	Date     string `json:"date,omitempty"`
	TimeZone string `json:"time_zone,omitempty"`
}

type Attendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"display_name,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
}

type EventPerson struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Self        bool   `json:"self,omitempty"`
}
