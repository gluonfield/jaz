package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessioncontext"
)

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(s.AuthKey)
		if key == "" || r.URL.Path == "/health" || internalMCPRequest(r) || publicDeviceRequest(r) || publicConnectionOAuthCallback(r) || localBrowserExtensionRequest(r) {
			next.ServeHTTP(w, r.WithContext(contextWithClientInfo(r, deviceauth.Principal{})))
			return
		}
		token := requestAuthKey(r)
		if s.Devices != nil {
			info := httpapi.RequestInfoFrom(r)
			principal, err := s.Devices.Authenticate(token, deviceauth.SeenInfo{IP: info.IP, UserAgent: info.UserAgent})
			if err == nil {
				next.ServeHTTP(w, r.WithContext(contextWithClientInfo(r, principal)))
				return
			}
			if errors.Is(err, deviceauth.ErrApprovalRequired) {
				writeAuthError(w, http.StatusForbidden, "device_approval_required", err)
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(key)) == 1 {
			if rootKeyAllowed(r) || s.rootKeyHasFullAccess() {
				principal := deviceauth.Principal{Kind: deviceauth.PrincipalRoot}
				next.ServeHTTP(w, r.WithContext(contextWithClientInfo(r, principal)))
				return
			}
			writeAuthError(w, http.StatusForbidden, "device_approval_required", deviceauth.ErrApprovalRequired)
			return
		}
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", fmt.Errorf("missing or invalid backend API key"))
	})
}

const clientPlatformHeader = "X-Jaz-Client-Platform"

func contextWithClientInfo(r *http.Request, principal deviceauth.Principal) context.Context {
	ctx := r.Context()
	if principal.Kind != "" {
		ctx = deviceauth.WithPrincipal(ctx, principal)
	}
	return sessioncontext.WithClientPlatform(ctx, requestClientPlatform(r))
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

func (s *Server) rootKeyHasFullAccess() bool {
	if s.Devices == nil {
		return true
	}
	count, err := s.Devices.ApprovedDeviceCount()
	return err != nil || count == 0
}

func rootKeyAllowed(r *http.Request) bool {
	if r.Method == http.MethodPost && r.URL.Path == "/v1/devices/register" {
		return true
	}
	return r.Method == http.MethodGet && r.URL.Path == "/v1/browser/extension"
}

func localBrowserExtensionRequest(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		r.URL.Path == "/v1/browser/extension" &&
		loopbackRequest(r) &&
		strings.HasPrefix(strings.TrimSpace(r.Header.Get("Origin")), "chrome-extension://")
}

func publicDeviceRequest(r *http.Request) bool {
	if r.Method == http.MethodPost && r.URL.Path == "/v1/devices/pairing-requests" {
		return true
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/devices/pairing-requests/") {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/devices/pairing-requests/"), "/")
		return rest != "" && !strings.Contains(rest, "/")
	}
	return false
}

func publicConnectionOAuthCallback(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/v1/connections/oauth/google/callback"
}

func writeAuthError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error(), "code": code})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	principal, _ := deviceauth.PrincipalFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"auth_kind": principal.Kind,
		"device_id": principal.DeviceID,
	})
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
	if r.URL.Path == "/v1/browser/extension" {
		return true
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
	case serverconfig.JazToolsMCPPath, serverconfig.JazToolsMCPCompatPath, serverconfig.JazmemMCPPath, serverconfig.MCPProxyPath:
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
