package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const APIBaseURL = "https://www.googleapis.com"

type APIClient struct {
	HTTP    *http.Client
	BaseURL string
}

type APIError struct {
	StatusCode int
	Status     string
	Message    string
	Reason     string
}

func (e APIError) Error() string {
	if e.Reason == "SERVICE_DISABLED" || e.Reason == "accessNotConfigured" {
		return "google calendar api is disabled for the OAuth client project; configure a Calendar-enabled Google OAuth client and reconnect Google Calendar"
	}
	if e.Message != "" {
		return "google calendar api: " + clampErrorMessage(e.Message)
	}
	if e.Status != "" {
		return "google calendar api: " + e.Status
	}
	return "google calendar api request failed"
}

func (c APIClient) UserInfo(ctx context.Context) (UserInfo, error) {
	var info UserInfo
	if err := c.get(ctx, "oauth2/v2/userinfo", nil, &info); err != nil {
		return UserInfo{}, err
	}
	if strings.TrimSpace(info.Email) == "" {
		return UserInfo{}, fmt.Errorf("google userinfo returned no email address")
	}
	return info, nil
}

func (c APIClient) GetEvents(ctx context.Context, input GetEventsRequest) (GetEventsResponse, error) {
	calendarID := cleanCalendarID(input.CalendarID)
	q := url.Values{}
	q.Set("singleEvents", "true")
	q.Set("orderBy", "startTime")
	if !input.TimeMin.IsZero() {
		q.Set("timeMin", input.TimeMin.Format(time.RFC3339Nano))
	}
	if !input.TimeMax.IsZero() {
		q.Set("timeMax", input.TimeMax.Format(time.RFC3339Nano))
	}
	if input.Query != "" {
		q.Set("q", input.Query)
	}
	if input.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(input.MaxResults))
	}
	if input.PageToken != "" {
		q.Set("pageToken", input.PageToken)
	}
	var list eventList
	if err := c.get(ctx, "calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events", q, &list); err != nil {
		return GetEventsResponse{}, err
	}
	out := GetEventsResponse{
		CalendarID:    calendarID,
		Events:        make([]Event, 0, len(list.Items)),
		NextPageToken: list.NextPageToken,
	}
	for _, item := range list.Items {
		out.Events = append(out.Events, eventFromAPI(calendarID, item))
	}
	return out, nil
}

func (c APIClient) CreateEvent(ctx context.Context, input CreateEventRequest) (Event, error) {
	calendarID := cleanCalendarID(input.CalendarID)
	body, err := eventInsertBody(input)
	if err != nil {
		return Event{}, err
	}
	sendUpdates, err := NormalizeSendUpdates(input.SendUpdates, len(body.Attendees))
	if err != nil {
		return Event{}, err
	}
	q := url.Values{}
	if sendUpdates != "" {
		q.Set("sendUpdates", sendUpdates)
	}
	var event apiEvent
	if err := c.post(ctx, "calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events", q, body, &event); err != nil {
		return Event{}, err
	}
	return eventFromAPI(calendarID, event), nil
}

func eventInsertBody(input CreateEventRequest) (apiEvent, error) {
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		return apiEvent{}, fmt.Errorf("summary is required")
	}
	start, err := eventTimeInput(input.Start, "start")
	if err != nil {
		return apiEvent{}, err
	}
	end, err := eventTimeInput(input.End, "end")
	if err != nil {
		return apiEvent{}, err
	}
	attendees := make([]apiAttendee, 0, len(input.Attendees))
	for _, attendee := range input.Attendees {
		email := strings.TrimSpace(attendee.Email)
		if email == "" {
			continue
		}
		attendees = append(attendees, apiAttendee{
			Email:       email,
			DisplayName: strings.TrimSpace(attendee.DisplayName),
			Optional:    attendee.Optional,
		})
	}
	return apiEvent{
		Summary:     summary,
		Description: strings.TrimSpace(input.Description),
		Location:    strings.TrimSpace(input.Location),
		Start:       start,
		End:         end,
		Attendees:   attendees,
	}, nil
}

func eventTimeInput(input EventTimeInput, field string) (apiEventTime, error) {
	date := strings.TrimSpace(input.Date)
	timeZone := strings.TrimSpace(input.TimeZone)
	if !input.DateTime.IsZero() {
		return apiEventTime{DateTime: input.DateTime.Format(time.RFC3339Nano), TimeZone: timeZone}, nil
	}
	if date == "" {
		return apiEventTime{}, fmt.Errorf("%s is required", field)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return apiEventTime{}, fmt.Errorf("%s date must use YYYY-MM-DD", field)
	}
	return apiEventTime{Date: date}, nil
}

func NormalizeSendUpdates(value string, attendeeCount int) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		if attendeeCount > 0 {
			return "all", nil
		}
		return "", nil
	case "all":
		return "all", nil
	case "external_only", "externalonly":
		return "externalOnly", nil
	case "none":
		return "none", nil
	default:
		return "", fmt.Errorf("send_updates must be all, external_only, or none")
	}
}

func cleanCalendarID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return CalendarIDPrimary
	}
	return id
}

func (c APIClient) get(ctx context.Context, path string, query url.Values, out any) error {
	endpoint, err := url.JoinPath(c.baseURL(), path)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return apiError(res, body)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c APIClient) post(ctx context.Context, path string, query url.Values, body any, out any) error {
	endpoint, err := url.JoinPath(c.baseURL(), path)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return apiError(res, body)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func apiError(res *http.Response, body []byte) error {
	out := APIError{StatusCode: res.StatusCode, Status: res.Status}
	var parsed googleErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil {
		out.Message = parsed.Error.Message
		out.Reason = parsed.reason()
	}
	if out.Message == "" {
		out.Message = strings.TrimSpace(string(body))
	}
	return out
}

type googleErrorResponse struct {
	Error struct {
		Message string              `json:"message"`
		Errors  []googleErrorReason `json:"errors"`
		Details []googleErrorReason `json:"details"`
	} `json:"error"`
}

type googleErrorReason struct {
	Reason string `json:"reason"`
}

func (r googleErrorResponse) reason() string {
	for _, detail := range r.Error.Details {
		if detail.Reason != "" {
			return detail.Reason
		}
	}
	for _, item := range r.Error.Errors {
		if item.Reason != "" {
			return item.Reason
		}
	}
	return ""
}

func clampErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len([]rune(message)) <= 240 {
		return message
	}
	return string([]rune(message)[:240]) + "..."
}

func (c APIClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c APIClient) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return APIBaseURL
}
