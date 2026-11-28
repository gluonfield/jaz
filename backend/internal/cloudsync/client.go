package cloudsync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

var (
	cgnatPrefix     = netip.MustParsePrefix("100.64.0.0/10")
	ietfProtoPrefix = netip.MustParsePrefix("192.0.0.0/24")
	benchPrefix     = netip.MustParsePrefix("198.18.0.0/15")
	reservedV4      = netip.MustParsePrefix("240.0.0.0/4")
	discardV6       = netip.MustParsePrefix("100::/64")
)

type fileLoopbackHostKey struct{}

type cloudClient struct {
	http          *http.Client
	stream        *http.Client
	fileTransport *http.Transport
	accountID     string
	refresh       func(ctx context.Context, accountID, failedToken string) (string, error)
}

func newCloudClient() *cloudClient {
	noRedirect := func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cloudClient{
		http:          &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect},
		stream:        &http.Client{Timeout: 0, CheckRedirect: noRedirect},
		fileTransport: newSafeFileTransport(),
	}
}

func newSafeFileTransport() *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid file host", ErrInvalidURL)
		}
		allowedLoopback, _ := ctx.Value(fileLoopbackHostKey{}).(string)
		allowLoopback := allowedLoopback != "" && strings.EqualFold(addr, allowedLoopback)
		if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			if isBlockedRedirectAddr(ip, allowLoopback) {
				return nil, fmt.Errorf("%w: blocked file redirect address", ErrInvalidURL)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("%w: failed to resolve file host", ErrInvalidURL)
		}
		for _, ip := range ips {
			if isBlockedRedirectAddr(ip, allowLoopback) {
				return nil, fmt.Errorf("%w: file host resolved to blocked address", ErrInvalidURL)
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
	return tr
}

func isBlockedRedirectAddr(addr netip.Addr, allowLoopback bool) bool {
	if !addr.IsValid() {
		return true
	}
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return !allowLoopback
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	if addr.Is4() && (cgnatPrefix.Contains(addr) || ietfProtoPrefix.Contains(addr) || benchPrefix.Contains(addr) || reservedV4.Contains(addr)) {
		return true
	}
	return addr.Is6() && discardV6.Contains(addr)
}

func canonicalHostPort(u *url.URL) string {
	if u == nil || u.Host == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(net.JoinHostPort(u.Hostname(), port))
}

func validateRunFileRedirectTarget(baseParsed *url.URL, baseIsLoopback bool, target *url.URL) error {
	if target == nil || target.Host == "" || target.User != nil {
		return fmt.Errorf("%w: invalid file redirect target", ErrInvalidURL)
	}
	sameLoopbackOrigin := baseParsed != nil &&
		baseIsLoopback &&
		strings.EqualFold(target.Scheme, baseParsed.Scheme) &&
		strings.EqualFold(target.Host, baseParsed.Host)
	if !strings.EqualFold(target.Scheme, "https") && !sameLoopbackOrigin {
		return fmt.Errorf("%w: file redirect requires HTTPS", ErrInvalidURL)
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if host == "" || host == "metadata" || host == "metadata.google.internal" || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("%w: blocked file redirect host", ErrInvalidURL)
	}
	if ip, err := netip.ParseAddr(host); err == nil && isBlockedRedirectAddr(ip, sameLoopbackOrigin) {
		return fmt.Errorf("%w: blocked file redirect address", ErrInvalidURL)
	}
	return nil
}

func IsLoopbackURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	if u.Scheme != "https" && !(u.Scheme == "http" && IsLoopbackURL(trimmed)) {
		return "", ErrInvalidURL
	}
	host := strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}
	return u.Scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/"), nil
}

func validatePathSegment(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00") {
		return ErrInvalidPath
	}
	return nil
}

func cleanRelPath(relpath string) ([]string, error) {
	trimmed := strings.TrimLeft(relpath, "/")
	if trimmed == "" {
		return nil, ErrInvalidPath
	}
	parts := strings.Split(trimmed, "/")
	escaped := make([]string, len(parts))
	for i, seg := range parts {
		if err := validatePathSegment(seg); err != nil {
			return nil, err
		}
		escaped[i] = url.PathEscape(seg)
	}
	return escaped, nil
}

func (c *cloudClient) doJSON(ctx context.Context, method, baseURL, path, token string, reqBody any) (json.RawMessage, int, error) {
	return c.doJSONWithHeaders(ctx, method, baseURL, path, token, reqBody, nil)
}

func (c *cloudClient) doJSONWithHeaders(ctx context.Context, method, baseURL, path, token string, reqBody any, headers http.Header) (json.RawMessage, int, error) {
	raw, status, err := c.doJSONOnce(ctx, method, baseURL, path, token, reqBody, headers)
	if !isUpstreamUnauthorized(err) || c.refresh == nil || c.accountID == "" || token == "" {
		return raw, status, err
	}
	nextToken, rerr := c.refresh(ctx, c.accountID, token)
	if rerr != nil {
		return raw, status, err
	}
	return c.doJSONOnce(ctx, method, baseURL, path, nextToken, reqBody, headers)
}

func (c *cloudClient) doJSONOnce(ctx context.Context, method, baseURL, path, token string, reqBody any, headers http.Header) (json.RawMessage, int, error) {
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
	maps.Copy(req.Header, headers)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
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
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	OrgID       string          `json:"org_id"`
	OrgName     string          `json:"org_name"`
	OrgIcon     json.RawMessage `json:"org_icon"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	Role        string          `json:"role"`
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
		OrgIcon:     r.OrgIcon,
		Email:       r.Email,
		DisplayName: r.DisplayName,
		Role:        r.Role,
	}
}

type exchangeResponse struct {
	Token        string         `json:"token"`
	RefreshToken string         `json:"refresh_token"`
	User         authMeResponse `json:"user"`
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

func (c *cloudClient) refreshToken(ctx context.Context, baseURL, refreshToken string) (exchangeResponse, error) {
	raw, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/auth/refresh", "", map[string]string{
		"refresh_token": refreshToken,
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

func (c *cloudClient) logout(ctx context.Context, baseURL, token, refreshToken string) error {
	var body any
	if refreshToken != "" {
		body = map[string]string{"refresh_token": refreshToken}
	}
	_, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/auth/logout", token, body)
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

type CreateSpaceRequest struct {
	Name             string                     `json:"name"`
	Visibility       string                     `json:"visibility,omitempty"`
	MemberUserIDs    []string                   `json:"member_user_ids,omitempty"`
	DefaultGitRepos  []storage.CloudGitRepoSpec `json:"default_git_repos,omitzero"`
	DefaultGitRepo   string                     `json:"default_git_repo,omitempty"`
	DefaultGitBranch string                     `json:"default_git_branch,omitempty"`
}

func (c *cloudClient) createSpace(ctx context.Context, baseURL, token string, req CreateSpaceRequest) (storage.CloudSpace, error) {
	raw, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/spaces", token, req)
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
	if err := validatePathSegment(threadID); err != nil {
		return storage.CloudThread{}, err
	}
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
	raw, _, err := c.doJSON(ctx, http.MethodPost, baseURL, "/v1/threads", token, req)
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

func instructionPath(scope, scopeID, kind string) (string, error) {
	for _, seg := range []string{scope, scopeID, kind} {
		if err := validatePathSegment(seg); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("/v1/instructions/%s/%s/%s", url.PathEscape(scope), url.PathEscape(scopeID), url.PathEscape(kind)), nil
}

func (c *cloudClient) getInstruction(ctx context.Context, baseURL, token, scope, scopeID, kind string) (storage.CloudInstruction, error) {
	path, err := instructionPath(scope, scopeID, kind)
	if err != nil {
		return storage.CloudInstruction{}, err
	}
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
	path, err := instructionPath(scope, scopeID, kind)
	if err != nil {
		return storage.CloudInstruction{}, err
	}
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

func (c *cloudClient) openEvents(ctx context.Context, client *http.Client, baseURL, token, threadID, query string) (*http.Response, error) {
	path := fmt.Sprintf("/v1/threads/%s/events?%s", url.PathEscape(threadID), query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return client.Do(req)
}

func (c *cloudClient) readEvents(ctx context.Context, client *http.Client, baseURL, token, threadID, query string, onEvent func(storage.CloudThreadEvent) error) error {
	if err := validatePathSegment(threadID); err != nil {
		return err
	}
	resp, err := c.openEvents(ctx, client, baseURL, token, threadID, query)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized && c.refresh != nil && c.accountID != "" && token != "" {
		if nextToken, rerr := c.refresh(ctx, c.accountID, token); rerr == nil {
			_ = resp.Body.Close()
			resp, err = c.openEvents(ctx, client, baseURL, nextToken, threadID, query)
			if err != nil {
				return err
			}
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return parseUpstreamError(resp.StatusCode, raw)
	}
	return readSSEStream(resp.Body, threadID, onEvent)
}

func (c *cloudClient) fetchEventsOnce(ctx context.Context, baseURL, token, threadID string, afterSeq int64) ([]storage.CloudThreadEvent, error) {
	var events []storage.CloudThreadEvent
	err := c.readEvents(ctx, c.http, baseURL, token, threadID, fmt.Sprintf("once=1&after_seq=%d", afterSeq), func(ev storage.CloudThreadEvent) error {
		events = append(events, ev)
		return nil
	})
	return events, err
}

func (c *cloudClient) streamEvents(ctx context.Context, baseURL, token, threadID string, afterSeq int64, onEvent func(storage.CloudThreadEvent) error) error {
	return c.readEvents(ctx, c.stream, baseURL, token, threadID, fmt.Sprintf("after_seq=%d", afterSeq), onEvent)
}

func (c *cloudClient) openRunFile(ctx context.Context, baseURL, token, runID, relpath string) (*http.Response, error) {
	if err := validatePathSegment(runID); err != nil {
		return nil, err
	}
	segments, err := cleanRelPath(relpath)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/v1/runs/%s/files/%s", url.PathEscape(runID), strings.Join(segments, "/"))
	return c.openCloudFilePath(ctx, baseURL, token, path)
}

func (c *cloudClient) openSpaceSourceContent(ctx context.Context, baseURL, token, spaceID, sourceID string) (*http.Response, error) {
	if err := validatePathSegment(spaceID); err != nil {
		return nil, err
	}
	if err := validatePathSegment(sourceID); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/v1/spaces/%s/sources/%s/content", url.PathEscape(spaceID), url.PathEscape(sourceID))
	return c.openCloudFilePath(ctx, baseURL, token, path)
}

func (c *cloudClient) openCloudFilePath(ctx context.Context, baseURL, token, path string) (*http.Response, error) {
	resp, err := c.openCloudFilePathOnce(ctx, baseURL, token, path)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || c.refresh == nil || c.accountID == "" || token == "" {
		return resp, err
	}
	nextToken, rerr := c.refresh(ctx, c.accountID, token)
	if rerr != nil {
		return resp, nil
	}
	_ = resp.Body.Close()
	return c.openCloudFilePathOnce(ctx, baseURL, nextToken, path)
}

func (c *cloudClient) openCloudFilePathOnce(ctx context.Context, baseURL, token, path string) (*http.Response, error) {
	baseParsed, _ := url.Parse(baseURL)
	baseIsLoopback := IsLoopbackURL(baseURL)
	reqCtx := ctx
	if baseIsLoopback {
		reqCtx = context.WithValue(ctx, fileLoopbackHostKey{}, canonicalHostPort(baseParsed))
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	fileClient := &http.Client{
		Timeout:   60 * time.Second,
		Transport: c.fileTransport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return fmt.Errorf("%w: too many file preview redirects", ErrInvalidURL)
			}
			if err := validateRunFileRedirectTarget(baseParsed, baseIsLoopback, next.URL); err != nil {
				return err
			}
			if baseParsed == nil || !strings.EqualFold(next.URL.Host, baseParsed.Host) || !strings.EqualFold(next.URL.Scheme, baseParsed.Scheme) {
				next.Header.Del("Authorization")
			}
			return nil
		},
	}
	return fileClient.Do(req)
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
