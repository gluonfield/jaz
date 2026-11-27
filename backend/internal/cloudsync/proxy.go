package cloudsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

const (
	maxProxyRequestBytes  int64 = 4 << 20
	maxProxyResponseBytes int64 = 16 << 20
)

func cleanProxyPath(raw string) (string, error) {
	if !strings.HasPrefix(raw, "/v1/") || strings.ContainsAny(raw, "\\\x00\r\n") {
		return "", ErrInvalidPath
	}
	parts := strings.Split(strings.TrimPrefix(raw, "/"), "/")
	escaped := make([]string, len(parts))
	for i, seg := range parts {
		if err := validatePathSegment(seg); err != nil {
			return "", ErrInvalidPath
		}
		escaped[i] = url.PathEscape(seg)
	}
	return "/" + strings.Join(escaped, "/"), nil
}

func sanitizeProxyQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ""
	}
	for k := range vals {
		if strings.EqualFold(k, "ticket") {
			delete(vals, k)
		}
	}
	return vals.Encode()
}

func isWidgetContentPath(path string) bool {
	return strings.HasPrefix(path, "/v1/widgets/") && strings.HasSuffix(path, "/content")
}

func writeProxyJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// On the local API a 401 coded "unauthorized" means the local key was rejected,
// so Cloud's own 401 body must never be relayed under it.
func cloudSessionRejected(w http.ResponseWriter, resp *http.Response) bool {
	if resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	writeProxyJSONError(w, http.StatusUnauthorized, ErrNotConnected.Error())
	return true
}

type preparedProxy struct {
	acc    storage.CloudAccount
	target string
	raw    []byte
	sse    bool
}

func (p preparedProxy) buildRequest(r *http.Request, token string) (*http.Request, error) {
	var body io.Reader
	if len(p.raw) > 0 {
		body = bytes.NewReader(p.raw)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, p.target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if p.sse {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		if accHdr := strings.TrimSpace(r.Header.Get("Accept")); accHdr != "" {
			req.Header.Set("Accept", accHdr)
		}
	}
	return req, nil
}

func (s *Service) prepareProxy(w http.ResponseWriter, r *http.Request, sse bool) (*preparedProxy, bool) {
	if s.ActiveWorkspaceID() == LocalWorkspaceID {
		return nil, false
	}
	cleanPath, err := cleanProxyPath(r.URL.Path)
	if err != nil {
		writeProxyJSONError(w, http.StatusBadRequest, "invalid cloud request path")
		return nil, true
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		writeProxyJSONError(w, http.StatusUnauthorized, err.Error())
		return nil, true
	}
	target := acc.BaseURL + cleanPath
	if q := sanitizeProxyQuery(r.URL.RawQuery); q != "" {
		target += "?" + q
	}
	var raw []byte
	if !sse && r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete) {
		raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, maxProxyRequestBytes))
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeProxyJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			} else {
				writeProxyJSONError(w, http.StatusBadRequest, "failed to read request body")
			}
			return nil, true
		}
	}
	return &preparedProxy{acc: acc, target: target, raw: raw, sse: sse}, true
}

// ProxyActiveWorkspace transparently forwards a request to the active Cloud
// workspace when one is selected, returning false immediately for Local mode.
func (s *Service) ProxyActiveWorkspace(w http.ResponseWriter, r *http.Request) bool {
	prepared, handled := s.prepareProxy(w, r, false)
	if !handled {
		return false
	}
	if prepared == nil {
		return true
	}
	resp, err := s.doAccountHTTP(r.Context(), prepared.acc, func(token string) (*http.Response, error) {
		req, err := prepared.buildRequest(r, token)
		if err != nil {
			return nil, err
		}
		return s.client.http.Do(req)
	})
	if err != nil {
		writeProxyJSONError(w, http.StatusBadGateway, "cloud request failed")
		return true
	}
	defer resp.Body.Close()
	if cloudSessionRejected(w, resp) {
		return true
	}

	for _, hdr := range []string{"Content-Type", "Content-Disposition", "ETag", "Cache-Control"} {
		if v := resp.Header.Get(hdr); v != "" {
			w.Header().Set(hdr, v)
		}
	}
	if isWidgetContentPath(r.URL.Path) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	}
	w.WriteHeader(resp.StatusCode)
	if resp.StatusCode != http.StatusNoContent {
		_, _ = io.Copy(w, io.LimitReader(resp.Body, maxProxyResponseBytes))
	}
	return true
}

// ProxyActiveWorkspaceSSE streams an SSE endpoint from the active Cloud
// workspace, returning false immediately for Local mode.
func (s *Service) ProxyActiveWorkspaceSSE(w http.ResponseWriter, r *http.Request) bool {
	prepared, handled := s.prepareProxy(w, r, true)
	if !handled {
		return false
	}
	if prepared == nil {
		return true
	}
	resp, err := s.doAccountHTTP(r.Context(), prepared.acc, func(token string) (*http.Response, error) {
		req, err := prepared.buildRequest(r, token)
		if err != nil {
			return nil, err
		}
		return s.client.stream.Do(req)
	})
	if err != nil {
		writeProxyJSONError(w, http.StatusBadGateway, "cloud event stream failed")
		return true
	}
	defer resp.Body.Close()
	if cloudSessionRejected(w, resp) {
		return true
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 1<<20))
		return true
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	controller := http.NewResponseController(w)
	w.WriteHeader(http.StatusOK)
	_ = controller.Flush()

	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return true
			}
			_ = controller.Flush()
		}
		if readErr != nil {
			return true
		}
	}
}
