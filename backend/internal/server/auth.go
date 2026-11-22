package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/sessioncontext"
)

const (
	clientPlatformHeader = "X-Jaz-Client-Platform"
	resourceTicketTTL    = time.Hour
	maxResourceTickets   = 1024
)

type ResourceTicket struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type resourceTicketStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func (s *resourceTicketStore) issue(now time.Time) (ResourceTicket, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ResourceTicket{}, err
	}
	token := "tk_" + base64.RawURLEncoding.EncodeToString(raw[:])
	expires := now.Add(resourceTicketTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]time.Time)
	}
	for k, exp := range s.entries {
		if !now.Before(exp) {
			delete(s.entries, k)
		}
	}
	if len(s.entries) >= maxResourceTickets {
		var oldestKey string
		var oldestExp time.Time
		for k, exp := range s.entries {
			if oldestKey == "" || exp.Before(oldestExp) {
				oldestKey = k
				oldestExp = exp
			}
		}
		if oldestKey != "" {
			delete(s.entries, oldestKey)
		}
	}
	s.entries[token] = expires
	return ResourceTicket{Token: token, ExpiresAt: expires}, nil
}

func (s *resourceTicketStore) valid(token string, now time.Time) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.entries[token]
	if !ok {
		return false
	}
	if !now.Before(expires) {
		delete(s.entries, token)
		return false
	}
	return true
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := sessioncontext.WithClientPlatform(r.Context(), requestClientPlatform(r))
		r = r.WithContext(ctx)
		key := s.AuthKey
		if key == "" || isPublicRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		if queryAuthAllowed(r) {
			if ticket := r.URL.Query().Get("ticket"); ticket != "" {
				if s.tickets.valid(ticket, time.Now().UTC()) {
					next.ServeHTTP(w, r)
					return
				}
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", fmt.Errorf("invalid or expired resource ticket"))
				return
			}
		}
		token := requestAuthKey(r)
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", fmt.Errorf("missing or invalid backend API key"))
	})
}

func isPublicRoute(r *http.Request) bool {
	if r.Method == http.MethodGet && (r.URL.Path == "/health" || r.URL.Path == "/.well-known/jaz-server") {
		return true
	}
	return publicOAuthCallback(r)
}

func requestClientPlatform(r *http.Request) string {
	if platform := strings.TrimSpace(r.Header.Get(clientPlatformHeader)); platform != "" {
		switch platform {
		case "browser", "cli", "desktop", "mobile":
			return platform
		}
	}
	return "desktop"
}

func publicOAuthCallback(r *http.Request) bool {
	return r.Method == http.MethodGet && (r.URL.Path == plugins.OAuthCallbackPath || r.URL.Path == mcpOAuthCallbackPath || r.URL.Path == m365copilot.OAuthCallbackPath)
}

func writeAuthError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error(), "code": code})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	authKind := "none"
	response := map[string]any{"ok": true}
	if s.AuthKey != "" {
		authKind = "bearer"
		ticket, err := s.tickets.issue(time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		response["resource_ticket"] = ticket
	}
	response["auth_kind"] = authKind
	writeJSON(w, http.StatusOK, response)
}

func requestAuthKey(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		for _, protocol := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
			protocol = strings.TrimSpace(protocol)
			if strings.HasPrefix(protocol, "jaz-auth.") {
				return strings.TrimPrefix(protocol, "jaz-auth.")
			}
		}
	}
	return ""
}

func queryAuthAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.URL.Path == "/v1/bots/events" {
		return true
	}
	sessionPath := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	if sessionPath == r.URL.Path {
		return false
	}
	sessionRef, action, ok := strings.Cut(sessionPath, "/")
	if !ok || sessionRef == "" {
		return false
	}
	return action == "events" ||
		strings.HasPrefix(action, "attachments/") ||
		(action == "file" && rawSessionFileRequested(r))
}

func bearerToken(header string) string {
	fields := strings.Fields(header)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return fields[1]
	}
	return ""
}
