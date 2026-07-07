package preview

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/httpapi"
)

const pathPrefix = "/v1/preview/"
const hostPrefix = "jaz-preview-"
const proxyTTL = 8 * time.Hour

type Handler struct {
	mu       sync.RWMutex
	proxies  map[string]proxyEntry
	proxyIDs map[string]string
}

type proxyEntry struct {
	origin    *url.URL
	expiresAt time.Time
}

func NewHandler() *Handler {
	return &Handler{proxies: make(map[string]proxyEntry), proxyIDs: make(map[string]string)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if id, ok := HostID(r.Host); ok {
		h.serveProxyID(w, r, id, r.URL.EscapedPath())
		return
	}
	if r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/preview/proxies" {
		h.createProxy(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpapi.WriteError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	rest, err := previewRest(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, err)
		return
	}
	switch {
	case strings.HasPrefix(rest, "p/"):
		h.servePathProxy(w, r, strings.TrimPrefix(rest, "p/"))
	default:
		httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func previewRest(r *http.Request) (string, error) {
	escaped := r.URL.EscapedPath()
	if !strings.HasPrefix(escaped, pathPrefix) {
		return "", fmt.Errorf("not found")
	}
	rest := strings.TrimPrefix(escaped, pathPrefix)
	if rest == "" {
		return "", fmt.Errorf("not found")
	}
	return rest, nil
}

func IsPublicHostRequest(r *http.Request) bool {
	_, ok := HostID(r.Host)
	return ok
}

func IsPublicPathRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return strings.HasPrefix(r.URL.EscapedPath(), "/v1/preview/p/")
}

func HostID(hostport string) (string, bool) {
	host := hostport
	if parsed, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsed
	}
	label, _, ok := strings.Cut(strings.ToLower(strings.Trim(host, "[]")), ".")
	if !ok || !strings.HasPrefix(label, hostPrefix) {
		return "", false
	}
	id := strings.TrimPrefix(label, hostPrefix)
	if id == "" {
		return "", false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return "", false
		}
	}
	return id, true
}

type createProxyRequest struct {
	URL string `json:"url"`
}

type createProxyResponse struct {
	URL         string `json:"url"`
	FallbackURL string `json:"fallback_url"`
}

func (h *Handler) createProxy(w http.ResponseWriter, r *http.Request) {
	var input createProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("read proxy request: %w", err))
		return
	}
	target, err := parseProxyTarget(input.URL)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	origin := proxyOriginURL(target)
	key := origin.String()
	now := time.Now()
	h.mu.Lock()
	h.pruneLocked(now)
	id := h.proxyIDs[key]
	if id == "" {
		var err error
		id, err = randomID()
		if err != nil {
			h.mu.Unlock()
			httpapi.WriteError(w, http.StatusInternalServerError, err)
			return
		}
	}
	h.proxies[id] = proxyEntry{origin: origin, expiresAt: now.Add(proxyTTL)}
	h.proxyIDs[key] = id
	h.mu.Unlock()
	fallback := pathProxyURL(r, id, target)
	resp := createProxyResponse{URL: hostProxyURL(r, id, target), FallbackURL: fallback}
	if resp.URL == "" {
		resp.URL = fallback
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func parseProxyTarget(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("preview proxy only supports http and https")
	}
	if u.User != nil || u.Host == "" {
		return nil, fmt.Errorf("proxy URL must include an origin")
	}
	if !loopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("preview proxy only supports server-local loopback URLs")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	if u.Hostname() == "0.0.0.0" {
		if port := u.Port(); port != "" {
			u.Host = net.JoinHostPort("127.0.0.1", port)
		} else {
			u.Host = "127.0.0.1"
		}
	}
	return u, nil
}

func proxyOriginURL(target *url.URL) *url.URL {
	return &url.URL{Scheme: target.Scheme, Host: target.Host}
}

func (h *Handler) proxyOrigin(id string) (*url.URL, bool) {
	now := time.Now()
	h.mu.RLock()
	entry, ok := h.proxies[id]
	h.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		h.mu.Lock()
		if latest, ok := h.proxies[id]; ok && now.After(latest.expiresAt) {
			h.deleteProxyLocked(id, latest.origin)
		}
		h.mu.Unlock()
		return nil, false
	}
	copy := *entry.origin
	return &copy, true
}

func (h *Handler) pruneLocked(now time.Time) {
	for id, entry := range h.proxies {
		if now.After(entry.expiresAt) {
			h.deleteProxyLocked(id, entry.origin)
		}
	}
}

func (h *Handler) deleteProxyLocked(id string, origin *url.URL) {
	delete(h.proxies, id)
	if h.proxyIDs[origin.String()] == id {
		delete(h.proxyIDs, origin.String())
	}
}

func hostProxyURL(r *http.Request, id string, target *url.URL) string {
	base, err := url.Parse(httpapi.RequestBaseURL(r))
	if err != nil || base.Host == "" {
		return ""
	}
	host, port := splitHostPort(base.Host)
	host = previewBaseHost(host)
	if host == "" {
		return ""
	}
	base.Host = hostPrefix + id + "." + host
	if port != "" {
		base.Host = net.JoinHostPort(base.Host, port)
	}
	base.Path = target.EscapedPath()
	base.RawQuery = target.RawQuery
	base.Fragment = target.Fragment
	return base.String()
}

func pathProxyURL(r *http.Request, id string, target *url.URL) string {
	base, err := url.Parse(httpapi.RequestBaseURL(r))
	if err != nil {
		return ""
	}
	base.Path = "/v1/preview/p/" + id + target.EscapedPath()
	base.RawQuery = target.RawQuery
	base.Fragment = target.Fragment
	return base.String()
}

func splitHostPort(hostport string) (string, string) {
	host, port, err := net.SplitHostPort(hostport)
	if err == nil {
		return host, port
	}
	return strings.Trim(hostport, "[]"), ""
}

func previewBaseHost(host string) string {
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return "localhost"
		}
		return ""
	}
	return host
}

func (h *Handler) servePathProxy(w http.ResponseWriter, r *http.Request, tail string) {
	id, suffix, ok := strings.Cut(tail, "/")
	if id == "" {
		httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("proxy id is required"))
		return
	}
	if !ok {
		suffix = ""
	}
	h.serveProxyID(w, r, id, "/"+suffix)
}

func (h *Handler) serveProxyID(w http.ResponseWriter, r *http.Request, id, escapedPath string) {
	target, ok := h.proxyOrigin(id)
	if !ok {
		httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("preview proxy not found"))
		return
	}
	h.proxy(w, r, target, escapedPath)
}

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request, target *url.URL, escapedPath string) {
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	proxy := &httputil.ReverseProxy{
		Director: func(out *http.Request) {
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.URL.Path = path
			if strings.Contains(escapedPath, "%") {
				out.URL.RawPath = escapedPath
			}
			out.URL.RawQuery = r.URL.RawQuery
			out.Host = target.Host
			out.Header.Del("Authorization")
			out.Header.Del("Cookie")
			out.Header.Del("Origin")
			out.Header.Del("Referer")
			out.Header.Del("X-Jaz-Client-Platform")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			httpapi.WriteError(w, http.StatusBadGateway, err)
		},
	}
	proxy.ServeHTTP(w, r)
}

func loopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
