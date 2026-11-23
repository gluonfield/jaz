package cloudsync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type cloudClient struct {
	http   *http.Client
	stream *http.Client
}

func newCloudClient() *cloudClient {
	return &cloudClient{
		http:   &http.Client{Timeout: 30 * time.Second},
		stream: &http.Client{Timeout: 0},
	}
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return "", ErrInvalidURL
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
	case "http":
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
		}
		return "", ErrInvalidURL
	default:
		return "", ErrInvalidURL
	}
}

func (c *cloudClient) doJSON(ctx context.Context, method, baseURL, path, token string, reqBody any) (json.RawMessage, int, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		switch v := reqBody.(type) {
		case json.RawMessage:
			bodyReader = bytes.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		default:
			encoded, err := json.Marshal(reqBody)
			if err != nil {
				return nil, 0, err
			}
			bodyReader = bytes.NewReader(encoded)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, parseUpstreamError(resp.StatusCode, raw)
	}
	return json.RawMessage(raw), resp.StatusCode, nil
}

func parseUpstreamError(status int, raw []byte) *UpstreamError {
	uerr := &UpstreamError{StatusCode: status}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		uerr.Body = json.RawMessage(trimmed)
		var parsed struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(trimmed, &parsed) == nil {
			if parsed.Error != "" {
				uerr.Message = parsed.Error
			} else if parsed.Message != "" {
				uerr.Message = parsed.Message
			}
		}
	}
	if uerr.Message == "" {
		uerr.Message = strings.TrimSpace(string(trimmed))
	}
	return uerr
}

type discoveryDoc struct {
	OK           bool            `json:"ok"`
	Mode         string          `json:"mode"`
	Auth         string          `json:"auth"`
	OIDCLoginURL string          `json:"oidc_login_url"`
	Capabilities json.RawMessage `json:"capabilities"`
}

func (c *cloudClient) discover(ctx context.Context, baseURL string) (discoveryDoc, error) {
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, "/.well-known/jaz-server", "", nil)
	if err != nil {
		return discoveryDoc{}, err
	}
	var doc discoveryDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return discoveryDoc{}, err
	}
	return doc, nil
}

type authMeResponse struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	OrgID       string `json:"org_id"`
	OrgName     string `json:"org_name"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

func (r authMeResponse) toUser() CloudUser {
	id := r.UserID
	if id == "" {
		id = r.ID
	}
	return CloudUser{
		ID:          id,
		UserID:      id,
		OrgID:       r.OrgID,
		OrgName:     r.OrgName,
		Email:       r.Email,
		DisplayName: r.DisplayName,
		Role:        r.Role,
	}
}

type exchangeResponse struct {
	Token string         `json:"token"`
	User  authMeResponse `json:"user"`
}

func (c *cloudClient) exchangeOIDC(ctx context.Context, baseURL, code, verifier string) (exchangeResponse, error) {
	raw, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/auth/exchange", "", map[string]string{
		"code":          code,
		"code_verifier": verifier,
	})
	if err != nil {
		return exchangeResponse{}, err
	}
	var out exchangeResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return exchangeResponse{}, err
	}
	return out, nil
}

func (c *cloudClient) getMe(ctx context.Context, baseURL, token string) (CloudUser, error) {
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, "/v1/auth/me", token, nil)
	if err != nil {
		return CloudUser{}, err
	}
	var parsed authMeResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CloudUser{}, err
	}
	return parsed.toUser(), nil
}

func (c *cloudClient) logout(ctx context.Context, baseURL, token string) error {
	_, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/auth/logout", token, nil)
	return err
}

func (c *cloudClient) listSpaces(ctx context.Context, baseURL, token string) ([]storage.CloudSpace, error) {
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, "/v1/spaces", token, nil)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []storage.CloudSpace{}, nil
	}
	if trimmed[0] == '[' {
		var direct []storage.CloudSpace
		if err := json.Unmarshal(trimmed, &direct); err != nil {
			return nil, err
		}
		return direct, nil
	}
	var wrapper struct {
		Spaces []storage.CloudSpace `json:"spaces"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Spaces == nil {
		return []storage.CloudSpace{}, nil
	}
	return wrapper.Spaces, nil
}

func (c *cloudClient) createSpace(ctx context.Context, baseURL, token, name string) (storage.CloudSpace, error) {
	raw, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/spaces", token, map[string]string{"name": name})
	if err != nil {
		return storage.CloudSpace{}, err
	}
	var sp storage.CloudSpace
	if err := json.Unmarshal(raw, &sp); err != nil {
		return storage.CloudSpace{}, err
	}
	return sp, nil
}

func (c *cloudClient) listThreads(ctx context.Context, baseURL, token string) ([]storage.CloudThread, error) {
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, "/v1/threads", token, nil)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []storage.CloudThread{}, nil
	}
	var items []wireThread
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
	} else {
		var wrapper struct {
			Threads []wireThread `json:"threads"`
		}
		if err := json.Unmarshal(trimmed, &wrapper); err != nil {
			return nil, err
		}
		items = wrapper.Threads
	}
	out := make([]storage.CloudThread, 0, len(items))
	for _, item := range items {
		out = append(out, item.toStorage())
	}
	return out, nil
}

func (c *cloudClient) getThread(ctx context.Context, baseURL, token, threadID string) (storage.CloudThread, error) {
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, "/v1/threads/"+url.PathEscape(threadID), token, nil)
	if err != nil {
		return storage.CloudThread{}, err
	}
	var item wireThread
	if err := json.Unmarshal(raw, &item); err != nil {
		return storage.CloudThread{}, err
	}
	return item.toStorage(), nil
}

func (c *cloudClient) createThread(ctx context.Context, baseURL, token string, req CreateThreadRequest) (storage.CloudThread, error) {
	payload := map[string]any{}
	if req.Title != "" {
		payload["title"] = req.Title
	}
	if req.SpaceID != "" {
		payload["space_id"] = req.SpaceID
	}
	if req.ModelProfileID != "" {
		payload["model_profile_id"] = req.ModelProfileID
	}
	if req.Model != "" {
		payload["model"] = req.Model
	}
	if req.ReasoningEffort != "" {
		payload["reasoning_effort"] = req.ReasoningEffort
	}
	if req.ExecutorTarget != "" {
		payload["executor_target"] = req.ExecutorTarget
	}
	raw, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/threads", token, payload)
	if err != nil {
		return storage.CloudThread{}, err
	}
	var item wireThread
	if err := json.Unmarshal(raw, &item); err != nil {
		return storage.CloudThread{}, err
	}
	th := item.toStorage()
	if th.ExecutorTarget == "" {
		th.ExecutorTarget = req.ExecutorTarget
	}
	return th, nil
}

func (c *cloudClient) getInstruction(ctx context.Context, baseURL, token, scope, scopeID, kind string) (storage.CloudInstruction, error) {
	path := fmt.Sprintf("/v1/instructions/%s/%s/%s", url.PathEscape(scope), url.PathEscape(scopeID), url.PathEscape(kind))
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, path, token, nil)
	if err != nil {
		return storage.CloudInstruction{}, err
	}
	var doc storage.CloudInstruction
	if err := json.Unmarshal(raw, &doc); err != nil {
		return storage.CloudInstruction{}, err
	}
	if doc.Scope == "" {
		doc.Scope = scope
	}
	if doc.ScopeID == "" {
		doc.ScopeID = scopeID
	}
	if doc.Kind == "" {
		doc.Kind = kind
	}
	return doc, nil
}

func (c *cloudClient) saveInstruction(ctx context.Context, baseURL, token, scope, scopeID, kind, bodyMD, expectedVersion string) (storage.CloudInstruction, error) {
	path := fmt.Sprintf("/v1/instructions/%s/%s/%s", url.PathEscape(scope), url.PathEscape(scopeID), url.PathEscape(kind))
	raw, _, err := c.doJSON(ctx, http.MethodPut, baseURL, path, token, map[string]string{
		"body_md":          bodyMD,
		"expected_version": expectedVersion,
	})
	if err != nil {
		return storage.CloudInstruction{}, err
	}
	var doc storage.CloudInstruction
	if err := json.Unmarshal(raw, &doc); err != nil {
		return storage.CloudInstruction{}, err
	}
	return doc, nil
}

func (c *cloudClient) listUsageEvents(ctx context.Context, baseURL, token string, afterID int64, limit int) ([]storage.CloudUsageEvent, error) {
	path := fmt.Sprintf("/v1/usage/events?after_id=%d&limit=%d", afterID, limit)
	raw, _, err := c.doJSON(ctx, http.MethodGet, baseURL, path, token, nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Events []storage.CloudUsageEvent `json:"events"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Events, nil
}

func (c *cloudClient) fetchEventsOnce(ctx context.Context, baseURL, token, threadID string, afterSeq int64) ([]storage.CloudThreadEvent, error) {
	path := fmt.Sprintf("/v1/threads/%s/events?once=1&after_seq=%d", url.PathEscape(threadID), afterSeq)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, parseUpstreamError(resp.StatusCode, raw)
	}
	var events []storage.CloudThreadEvent
	err = readSSEStream(resp.Body, threadID, func(ev storage.CloudThreadEvent) error {
		events = append(events, ev)
		return nil
	})
	return events, err
}

func (c *cloudClient) streamEvents(ctx context.Context, baseURL, token, threadID string, afterSeq int64, onEvent func(storage.CloudThreadEvent) error) error {
	path := fmt.Sprintf("/v1/threads/%s/events?after_seq=%d", url.PathEscape(threadID), afterSeq)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return parseUpstreamError(resp.StatusCode, raw)
	}
	return readSSEStream(resp.Body, threadID, onEvent)
}

func (c *cloudClient) openRunFile(ctx context.Context, baseURL, token, runID, relpath string) (*http.Response, error) {
	segments := strings.Split(strings.TrimLeft(relpath, "/"), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	path := fmt.Sprintf("/v1/runs/%s/files/%s", url.PathEscape(runID), strings.Join(segments, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return c.http.Do(req)
}

func readSSEStream(r io.Reader, threadID string, onEvent func(storage.CloudThreadEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	var eventName, eventID string
	var dataLines []string

	flush := func() error {
		if len(dataLines) == 0 {
			eventName, eventID = "", ""
			return nil
		}
		raw := strings.Join(dataLines, "\n")
		name, id := eventName, eventID
		eventName, eventID, dataLines = "", "", dataLines[:0]
		var w wireEvent
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			return nil
		}
		if w.Seq == 0 && id != "" {
			w.Seq, _ = strconv.ParseInt(id, 10, 64)
		}
		if w.Type == "" {
			if name != "" {
				w.Type = name
			} else {
				w.Type = "message"
			}
		}
		return onEvent(w.toStorage(threadID))
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if v, ok := strings.CutPrefix(line, "event:"); ok {
			eventName = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "id:"); ok {
			eventID = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimPrefix(v, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}
