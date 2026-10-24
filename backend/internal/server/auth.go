package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessioncontext"
)

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := s.AuthKey
		if key == "" || r.URL.Path == "/health" || publicDeviceRequest(r) || publicOAuthCallback(r) {
			next.ServeHTTP(w, r.WithContext(contextWithClientInfo(r, deviceauth.Principal{})))
			return
		}
		if s.Devices != nil && queryAuthAllowed(r) && r.URL.Query().Get("ticket") != "" {
			principal, err := s.Devices.AuthenticateResourceTicket(r.URL.Query().Get("ticket"))
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", err)
				return
			}
			s.serveDevice(w, r, principal, next)
			return
		}
		token := requestAuthKey(r)
		if s.Devices != nil && r.Method == http.MethodPost && r.URL.Path == "/v1/devices/register" && strings.HasPrefix(token, "invite_") {
			principal := deviceauth.Principal{Kind: deviceauth.PrincipalEnrollment, EnrollmentToken: token}
			next.ServeHTTP(w, r.WithContext(contextWithClientInfo(r, principal)))
			return
		}
		if s.Devices != nil {
			info := httpapi.RequestInfoFrom(r)
			principal, err := s.Devices.Authenticate(token, deviceauth.SeenInfo{IP: info.IP, UserAgent: info.UserAgent})
			if err == nil {
				s.serveDevice(w, r, principal, next)
				return
			}
			if errors.Is(err, deviceauth.ErrApprovalRequired) {
				writeAuthError(w, http.StatusForbidden, "device_approval_required", err)
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
			if s.Devices == nil || (trustedLocalRequest(r) && (rootKeyAllowed(r) || s.rootKeyHasFullAccess())) {
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
	return err == nil && count == 0
}

func rootKeyAllowed(r *http.Request) bool {
	return (r.Method == http.MethodPost && r.URL.Path == "/v1/devices/register") || internalMCPRequest(r)
}

func trustedLocalRequest(r *http.Request) bool {
	if !loopbackRequest(r) {
		return false
	}
	host := (&url.URL{Host: r.Host}).Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return false
	}
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "CF-Connecting-IP"} {
		if r.Header.Get(name) != "" {
			return false
		}
	}
	return true
}

func (s *Server) serveDevice(w http.ResponseWriter, r *http.Request, principal deviceauth.Principal, next http.Handler) {
	ctx, release, err := s.Devices.Track(contextWithClientInfo(r, principal), principal)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}
	defer release()
	next.ServeHTTP(w, r.WithContext(ctx))
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

func publicOAuthCallback(r *http.Request) bool {
	return r.Method == http.MethodGet && (r.URL.Path == "/v1/connections/oauth/callback" || r.URL.Path == mcpOAuthCallbackPath)
}

func writeAuthError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error(), "code": code})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	principal, _ := deviceauth.PrincipalFromContext(r.Context())
	response := map[string]any{
		"ok":        true,
		"auth_kind": principal.Kind,
		"device_id": principal.DeviceID,
	}
	if principal.Kind == deviceauth.PrincipalDevice {
		ticket, err := s.Devices.IssueResourceTicket(requestAuthKey(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		response["resource_ticket"] = ticket
	}
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
