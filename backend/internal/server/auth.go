package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/sessioncontext"
)

const (
	clientPlatformHeader  = "X-Jaz-Client-Platform"
	resourceTicketTTL     = time.Hour
	resourceTicketPurpose = "jaz-resource-ticket-v1:"
)

type ResourceTicket struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func issueResourceTicket(authKey string, now time.Time) ResourceTicket {
	expires := now.UTC().Add(resourceTicketTTL).Truncate(time.Second)
	payload := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(expires.Unix(), 10)))
	sig := signResourceTicket(authKey, payload)
	return ResourceTicket{
		Token:     "tk_" + payload + "." + sig,
		ExpiresAt: expires,
	}
}

func validResourceTicket(authKey, token string, now time.Time) bool {
	if authKey == "" {
		return false
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(token), "tk_")
	if !ok {
		return false
	}
	payload, sig, ok := strings.Cut(rest, ".")
	if !ok || payload == "" || sig == "" {
		return false
	}
	expected := signResourceTicket(authKey, payload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return false
	}
	rawExp, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return false
	}
	expUnix, err := strconv.ParseInt(string(rawExp), 10, 64)
	if err != nil {
		return false
	}
	return now.UTC().Before(time.Unix(expUnix, 0).UTC())
}

func signResourceTicket(authKey, payload string) string {
	mac := hmac.New(sha256.New, []byte(authKey))
	_, _ = mac.Write([]byte(resourceTicketPurpose))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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
				if validResourceTicket(key, ticket, time.Now().UTC()) {
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
		response["resource_ticket"] = issueResourceTicket(s.AuthKey, time.Now().UTC())
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
