package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const APIBaseURL = "https://gmail.googleapis.com"

type Profile struct {
	EmailAddress  string `json:"emailAddress"`
	MessagesTotal int64  `json:"messagesTotal"`
	ThreadsTotal  int64  `json:"threadsTotal"`
	HistoryID     string `json:"historyId"`
}

type SearchMessagesRequest struct {
	Query      string
	MaxResults int
}

type SearchMessagesResponse struct {
	Messages           []Message `json:"messages"`
	NextPageToken      string    `json:"next_page_token,omitempty"`
	ResultSizeEstimate int64     `json:"result_size_estimate,omitempty"`
}

type MessageContent struct {
	Message  Message `json:"message"`
	BodyText string  `json:"body_text,omitempty"`
	BodyHTML string  `json:"body_html,omitempty"`
}

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
		return "gmail api is disabled for the OAuth client project; configure a Gmail-enabled Google OAuth client and reconnect Gmail"
	}
	if e.Message != "" {
		return "gmail api: " + clampErrorMessage(e.Message)
	}
	if e.Status != "" {
		return "gmail api: " + e.Status
	}
	return "gmail api request failed"
}

func (c APIClient) Profile(ctx context.Context) (Profile, error) {
	var profile Profile
	if err := c.get(ctx, "gmail/v1/users/me/profile", nil, &profile); err != nil {
		return Profile{}, err
	}
	if profile.EmailAddress == "" {
		return Profile{}, fmt.Errorf("gmail profile returned no email address")
	}
	return profile, nil
}

func (c APIClient) SearchMessages(ctx context.Context, input SearchMessagesRequest) (SearchMessagesResponse, error) {
	q := url.Values{}
	if input.Query != "" {
		q.Set("q", input.Query)
	}
	if input.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(input.MaxResults))
	}
	var list messageList
	if err := c.get(ctx, "gmail/v1/users/me/messages", q, &list); err != nil {
		return SearchMessagesResponse{}, err
	}
	out := SearchMessagesResponse{
		Messages:           make([]Message, 0, len(list.Messages)),
		NextPageToken:      list.NextPageToken,
		ResultSizeEstimate: list.ResultSizeEstimate,
	}
	for _, ref := range list.Messages {
		if ref.ID == "" {
			continue
		}
		message, err := c.message(ctx, ref.ID, "metadata")
		if err != nil {
			return SearchMessagesResponse{}, err
		}
		out.Messages = append(out.Messages, messageFromAPI(message))
	}
	return out, nil
}

func (c APIClient) ReadMessage(ctx context.Context, id string) (MessageContent, error) {
	if id == "" {
		return MessageContent{}, fmt.Errorf("gmail message id is required")
	}
	raw, err := c.message(ctx, id, "full")
	if err != nil {
		return MessageContent{}, err
	}
	text, html := messageBodies(raw.Payload)
	return MessageContent{
		Message:  messageFromAPI(raw),
		BodyText: text,
		BodyHTML: html,
	}, nil
}

func (c APIClient) message(ctx context.Context, id, format string) (apiMessage, error) {
	q := url.Values{}
	q.Set("format", format)
	if format == "metadata" {
		for _, header := range []string{"From", "To", "Cc", "Bcc", "Subject", "Date"} {
			q.Add("metadataHeaders", header)
		}
	}
	var message apiMessage
	return message, c.get(ctx, "gmail/v1/users/me/messages/"+id, q, &message)
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

type messageList struct {
	Messages           []messageRef `json:"messages"`
	NextPageToken      string       `json:"nextPageToken"`
	ResultSizeEstimate int64        `json:"resultSizeEstimate"`
}

type messageRef struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type apiMessage struct {
	ID           string      `json:"id"`
	ThreadID     string      `json:"threadId"`
	HistoryID    string      `json:"historyId"`
	Snippet      string      `json:"snippet"`
	LabelIDs     []string    `json:"labelIds"`
	InternalDate string      `json:"internalDate"`
	Payload      messagePart `json:"payload"`
}

type messagePart struct {
	MIMEType string          `json:"mimeType"`
	Filename string          `json:"filename"`
	Headers  []messageHeader `json:"headers"`
	Body     messageBody     `json:"body"`
	Parts    []messagePart   `json:"parts"`
}

type messageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type messageBody struct {
	Data         string `json:"data"`
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
}

func messageFromAPI(raw apiMessage) Message {
	headers := headersByName(raw.Payload.Headers)
	return Message{
		ID:           raw.ID,
		ThreadID:     raw.ThreadID,
		HistoryID:    raw.HistoryID,
		Subject:      headers["subject"],
		Snippet:      raw.Snippet,
		From:         parseAddresses(headers["from"]),
		To:           parseAddresses(headers["to"]),
		Cc:           parseAddresses(headers["cc"]),
		Bcc:          parseAddresses(headers["bcc"]),
		LabelIDs:     raw.LabelIDs,
		InternalDate: internalDate(raw.InternalDate),
		Attachments:  attachments(raw.Payload),
	}
}

func headersByName(headers []messageHeader) map[string]string {
	out := map[string]string{}
	for _, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header.Name))
		value := strings.TrimSpace(header.Value)
		if name != "" && value != "" {
			out[name] = value
		}
	}
	return out
}

func parseAddresses(value string) []Address {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return []Address{{Email: value}}
	}
	out := make([]Address, 0, len(parsed))
	for _, address := range parsed {
		out = append(out, Address{
			Name:  strings.TrimSpace(address.Name),
			Email: strings.TrimSpace(address.Address),
		})
	}
	return out
}

func internalDate(value string) time.Time {
	ms, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func attachments(part messagePart) []Attachment {
	var out []Attachment
	var walk func(messagePart)
	walk = func(part messagePart) {
		if part.Filename != "" || part.Body.AttachmentID != "" {
			out = append(out, Attachment{
				ID:       part.Body.AttachmentID,
				FileName: part.Filename,
				MIMEType: part.MIMEType,
				Size:     part.Body.Size,
				Inline:   part.Filename == "",
			})
		}
		for _, child := range part.Parts {
			walk(child)
		}
	}
	walk(part)
	return out
}

func messageBodies(part messagePart) (string, string) {
	var textParts []string
	var htmlParts []string
	var walk func(messagePart)
	walk = func(part messagePart) {
		switch strings.ToLower(strings.TrimSpace(part.MIMEType)) {
		case "text/plain":
			if text := decodeBody(part.Body.Data); text != "" {
				textParts = append(textParts, text)
			}
		case "text/html":
			if html := decodeBody(part.Body.Data); html != "" {
				htmlParts = append(htmlParts, html)
			}
		}
		for _, child := range part.Parts {
			walk(child)
		}
	}
	walk(part)
	return strings.Join(textParts, "\n\n"), strings.Join(htmlParts, "\n\n")
}

func decodeBody(data string) string {
	if data == "" {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(data)
	}
	if err != nil {
		return ""
	}
	return string(raw)
}
