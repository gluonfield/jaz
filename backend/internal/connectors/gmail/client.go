package gmail

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
)

const APIBaseURL = "https://gmail.googleapis.com"

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

func (c APIClient) SearchThreads(ctx context.Context, input SearchThreadsRequest) (SearchThreadsResponse, error) {
	q := url.Values{}
	if input.Query != "" {
		q.Set("q", input.Query)
	}
	if input.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(input.MaxResults))
	}
	var list threadList
	if err := c.get(ctx, "gmail/v1/users/me/threads", q, &list); err != nil {
		return SearchThreadsResponse{}, err
	}
	out := SearchThreadsResponse{
		Threads:            make([]Thread, 0, len(list.Threads)),
		NextPageToken:      list.NextPageToken,
		ResultSizeEstimate: list.ResultSizeEstimate,
	}
	for _, ref := range list.Threads {
		if ref.ID == "" {
			continue
		}
		thread, err := c.thread(ctx, ref.ID, "metadata")
		if err != nil {
			return SearchThreadsResponse{}, err
		}
		out.Threads = append(out.Threads, threadFromAPI(thread))
	}
	return out, nil
}

func (c APIClient) ReadThread(ctx context.Context, input ReadThreadRequest) (ThreadContent, error) {
	id := input.ID
	if id == "" {
		return ThreadContent{}, fmt.Errorf("gmail message or thread id is required")
	}
	if input.IDType != IDTypeThread {
		message, err := c.message(ctx, id, "metadata")
		if err != nil {
			return ThreadContent{}, err
		}
		id = message.ThreadID
	}
	if id == "" {
		return ThreadContent{}, fmt.Errorf("gmail thread id is required")
	}
	raw, err := c.thread(ctx, id, "full")
	if err != nil {
		return ThreadContent{}, err
	}
	content := threadContentFromAPI(raw)
	if input.MaxMessages > 0 && len(content.Messages) > input.MaxMessages {
		content.Messages = content.Messages[len(content.Messages)-input.MaxMessages:]
	}
	return content, nil
}

func (c APIClient) CreateDraft(ctx context.Context, input ComposeMessageRequest) (Draft, error) {
	request, err := draftRequest("", input)
	if err != nil {
		return Draft{}, err
	}
	var draft apiDraft
	if err := c.post(ctx, "gmail/v1/users/me/drafts", request, &draft); err != nil {
		return Draft{}, err
	}
	return draftFromAPI(draft), nil
}

func (c APIClient) GetDraft(ctx context.Context, id string) (DraftContent, error) {
	if id == "" {
		return DraftContent{}, fmt.Errorf("gmail draft id is required")
	}
	raw, err := c.draft(ctx, id, "full")
	if err != nil {
		return DraftContent{}, err
	}
	text, html := messageBodies(raw.Message.Payload)
	return DraftContent{
		Draft:    draftFromAPI(raw),
		BodyText: text,
		BodyHTML: html,
	}, nil
}

func (c APIClient) UpdateDraft(ctx context.Context, id string, input ComposeMessageRequest) (Draft, error) {
	request, err := draftRequest(id, input)
	if err != nil {
		return Draft{}, err
	}
	var draft apiDraft
	if err := c.put(ctx, "gmail/v1/users/me/drafts/"+id, request, &draft); err != nil {
		return Draft{}, err
	}
	return draftFromAPI(draft), nil
}

func (c APIClient) SendDraft(ctx context.Context, id string) (Message, error) {
	if id == "" {
		return Message{}, fmt.Errorf("gmail draft id is required")
	}
	var sent apiMessage
	if err := c.post(ctx, "gmail/v1/users/me/drafts/send", apiDraftRequest{ID: id}, &sent); err != nil {
		return Message{}, err
	}
	return messageFromAPI(sent), nil
}

func (c APIClient) ListDrafts(ctx context.Context, input ListDraftsRequest) (ListDraftsResponse, error) {
	q := url.Values{}
	if input.Query != "" {
		q.Set("q", input.Query)
	}
	if input.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(input.MaxResults))
	}
	var list draftList
	if err := c.get(ctx, "gmail/v1/users/me/drafts", q, &list); err != nil {
		return ListDraftsResponse{}, err
	}
	out := ListDraftsResponse{
		Drafts:             make([]Draft, 0, len(list.Drafts)),
		NextPageToken:      list.NextPageToken,
		ResultSizeEstimate: list.ResultSizeEstimate,
	}
	for _, ref := range list.Drafts {
		if ref.ID == "" {
			continue
		}
		draft, err := c.draft(ctx, ref.ID, "metadata")
		if err != nil {
			return ListDraftsResponse{}, err
		}
		out.Drafts = append(out.Drafts, draftFromAPI(draft))
	}
	return out, nil
}

func (c APIClient) message(ctx context.Context, id, format string) (apiMessage, error) {
	q := url.Values{}
	q.Set("format", format)
	if format == "metadata" {
		for _, header := range metadataHeaders() {
			q.Add("metadataHeaders", header)
		}
	}
	var message apiMessage
	return message, c.get(ctx, "gmail/v1/users/me/messages/"+id, q, &message)
}

func (c APIClient) thread(ctx context.Context, id, format string) (apiThread, error) {
	q := url.Values{}
	q.Set("format", format)
	if format == "metadata" {
		for _, header := range metadataHeaders() {
			q.Add("metadataHeaders", header)
		}
	}
	var thread apiThread
	return thread, c.get(ctx, "gmail/v1/users/me/threads/"+id, q, &thread)
}

func (c APIClient) draft(ctx context.Context, id, format string) (apiDraft, error) {
	q := url.Values{}
	q.Set("format", format)
	if format == "metadata" {
		for _, header := range metadataHeaders() {
			q.Add("metadataHeaders", header)
		}
	}
	var draft apiDraft
	return draft, c.get(ctx, "gmail/v1/users/me/drafts/"+id, q, &draft)
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

func (c APIClient) post(ctx context.Context, path string, body any, out any) error {
	return c.write(ctx, http.MethodPost, path, body, out)
}

func (c APIClient) put(ctx context.Context, path string, body any, out any) error {
	return c.write(ctx, http.MethodPut, path, body, out)
}

func (c APIClient) write(ctx context.Context, method, path string, body any, out any) error {
	endpoint, err := url.JoinPath(c.baseURL(), path)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
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
