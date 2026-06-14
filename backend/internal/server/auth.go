package server

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/serverconfig"
)

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(s.AuthKey)
		if key == "" || r.URL.Path == "/health" || internalMCPRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(requestAuthKey(r)), []byte(key)) != 1 {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("missing or invalid backend API key"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func requestAuthKey(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	if !queryAuthAllowed(r) {
		return ""
	}
	return strings.TrimSpace(r.URL.Query().Get("key"))
}

func queryAuthAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	sessionPath := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	if sessionPath == r.URL.Path {
		return false
	}
	sessionRef, action, ok := strings.Cut(sessionPath, "/")
	return ok && sessionRef != "" && (action == "events" || action == "terminal")
}

func bearerToken(header string) string {
	fields := strings.Fields(header)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return strings.TrimSpace(fields[1])
	}
	return ""
}

func internalMCPRequest(r *http.Request) bool {
	switch r.URL.Path {
	case serverconfig.JazToolsMCPPath, serverconfig.JazToolsMCPCompatPath, serverconfig.JazmemMCPPath:
		return loopbackRequest(r)
	default:
		return false
	}
}

func loopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
