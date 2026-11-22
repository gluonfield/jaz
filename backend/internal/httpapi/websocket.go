package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/gorilla/websocket"
)

type trustedOriginsKey struct{}

func WithTrustedOrigins(ctx context.Context, origins []string) context.Context {
	return context.WithValue(ctx, trustedOriginsKey{}, slices.Clone(origins))
}

func TrustedOriginsFromContext(ctx context.Context) []string {
	origins, _ := ctx.Value(trustedOriginsKey{}).([]string)
	return origins
}

// AllowedOrigin reports whether a browser Origin header is permitted for Jaz
// HTTP and WebSocket endpoints. Exact "file://" is permitted because packaged
// Electron renderers send Origin: file:// on WebSocket and remote requests;
// "null" origins are never permitted.
func AllowedOrigin(origin string, trusted ...string) bool {
	if origin == "file://" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	if origin == "https://web.jaz.chat" || origin == "https://jaz.chat" {
		return true
	}
	if host := u.Hostname(); host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	for _, candidate := range trusted {
		if candidate != "" && origin == candidate {
			return true
		}
	}
	return false
}

func UpgradeWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	// Negotiate a public protocol; authentication subprotocols remain request-only.
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"jaz"},
		CheckOrigin: func(req *http.Request) bool {
			origin := req.Header.Get("Origin")
			if origin == "" {
				return true
			}
			return AllowedOrigin(origin, TrustedOriginsFromContext(req.Context())...)
		},
	}
	return upgrader.Upgrade(w, r, nil)
}
