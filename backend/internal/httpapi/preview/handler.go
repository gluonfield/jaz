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
	"github.com/wins/jaz/backend/internal/serverconfig"
)

const probePath = "/.well-known/jaz-preview"
const proxyTTL = 8 * time.Hour

type Handler struct {
	mu              sync.RWMutex
	proxiesByOrigin map[string]proxyEntry
	proxiesByHost   map[string]proxyEntry
	originTemplate  *originTemplate
}

type proxyEntry struct {
	id        string
	origin    *url.URL
	host      string
	expiresAt time.Time
}

func NewHandler(config serverconfig.Config) (*Handler, error) {
	template, err := parseOriginTemplate(config.PreviewURLTemplate)
	if err != nil {
		return nil, err
	}
	return &Handler{
		proxiesByOrigin: make(map[string]proxyEntry),
		proxiesByHost:   make(map[string]proxyEntry),
		originTemplate:  template,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if proxy, ok := h.lookup(r.Host); ok {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.EscapedPath() == probePath {
			serveProbe(w)
			return
		}
		h.proxy(w, r, proxy.origin, r.URL.EscapedPath())
		return
	}
	if r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/preview/proxies" {
		h.createProxy(w, r)
		return
	}
	httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("not found"))
}

func (h *Handler) IsPublicHostRequest(r *http.Request) bool {
	_, ok := h.lookup(r.Host)
	return ok
}

type createProxyRequest struct {
	URL string `json:"url"`
}

type createProxyResponse struct {
	URL string `json:"url"`
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
	previous, ok := h.proxiesByOrigin[key]
	id := previous.id
	if !ok {
		var err error
		id, err = randomID()
		if err != nil {
			h.mu.Unlock()
			httpapi.WriteError(w, http.StatusInternalServerError, err)
			return
		}
	}
	source, err := h.previewURL(r, id, target)
	if err != nil {
		h.mu.Unlock()
		httpapi.WriteError(w, http.StatusServiceUnavailable, err)
		return
	}
	host, err := previewHost(source)
	if err != nil {
		h.mu.Unlock()
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if previous.host != "" && previous.host != host {
		delete(h.proxiesByHost, previous.host)
	}
	proxy := proxyEntry{id: id, origin: origin, host: host, expiresAt: now.Add(proxyTTL)}
	h.proxiesByOrigin[key] = proxy
	h.proxiesByHost[host] = proxy
	h.mu.Unlock()
	httpapi.WriteJSON(w, http.StatusOK, createProxyResponse{URL: source})
}

func (h *Handler) previewURL(r *http.Request, id string, target *url.URL) (string, error) {
	if h.originTemplate != nil {
		return h.originTemplate.previewURL(id, target), nil
	}
	if source, ok := localPreviewURL(r, id, target); ok {
		return source, nil
	}
	return "", fmt.Errorf("remote preview origin is not configured; set --preview-url-template to an isolated origin containing {id}")
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

func (h *Handler) lookup(host string) (proxyEntry, bool) {
	now := time.Now()
	host = canonicalHost(host)
	h.mu.RLock()
	entry, ok := h.proxiesByHost[host]
	h.mu.RUnlock()
	if !ok {
		return proxyEntry{}, false
	}
	if now.After(entry.expiresAt) {
		h.mu.Lock()
		entry, ok = h.proxiesByHost[host]
		if ok && now.After(entry.expiresAt) {
			h.deleteProxyLocked(entry)
			ok = false
		}
		h.mu.Unlock()
		if !ok {
			return proxyEntry{}, false
		}
	}
	copy := *entry.origin
	entry.origin = &copy
	return entry, true
}

func (h *Handler) pruneLocked(now time.Time) {
	for _, entry := range h.proxiesByOrigin {
		if now.After(entry.expiresAt) {
			h.deleteProxyLocked(entry)
		}
	}
}

func (h *Handler) deleteProxyLocked(proxy proxyEntry) {
	delete(h.proxiesByHost, proxy.host)
	delete(h.proxiesByOrigin, proxy.origin.String())
}

func serveProbe(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "X-Jaz-Preview")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Jaz-Preview", "ready")
	w.WriteHeader(http.StatusNoContent)
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
			stripCredentials(out.Header)
		},
		ModifyResponse: func(response *http.Response) error {
			rewriteLocation(response, target, httpapi.RequestBaseURL(r))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			httpapi.WriteError(w, http.StatusBadGateway, err)
		},
	}
	proxy.ServeHTTP(w, r)
}

func stripCredentials(header http.Header) {
	for name := range header {
		lower := strings.ToLower(name)
		if lower == "authorization" || lower == "proxy-authorization" || lower == "cookie" || lower == "origin" || lower == "referer" || strings.HasPrefix(lower, "x-jaz-") {
			header.Del(name)
		}
	}
}

func rewriteLocation(response *http.Response, target *url.URL, publicOrigin string) {
	raw := response.Header.Get("Location")
	location, err := url.Parse(raw)
	if err != nil || location.Host == "" || !strings.EqualFold(location.Host, target.Host) || (location.Scheme != "" && !strings.EqualFold(location.Scheme, target.Scheme)) {
		return
	}
	public, err := url.Parse(publicOrigin)
	if err != nil || public.Scheme == "" || public.Host == "" {
		return
	}
	location.Scheme = public.Scheme
	location.Host = public.Host
	response.Header.Set("Location", location.String())
}

func loopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
