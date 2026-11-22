package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestUpgradeWebSocketCheckOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithTrustedOrigins(r.Context(), []string{"https://self-hosted.example"}))
		conn, err := UpgradeWebSocket(w, r)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	for _, tc := range []struct {
		name   string
		origin string
		want   int
	}{
		{"native client omits origin", "", http.StatusSwitchingProtocols},
		{"packaged electron file origin", "file://", http.StatusSwitchingProtocols},
		{"hosted web origin", "https://web.jaz.chat", http.StatusSwitchingProtocols},
		{"loopback dev origin", "http://localhost:5173", http.StatusSwitchingProtocols},
		{"configured trusted origin", "https://self-hosted.example", http.StatusSwitchingProtocols},
		{"null origin rejected", "null", http.StatusForbidden},
		{"evil website rejected", "https://evil.example", http.StatusForbidden},
		{"file host rejected", "file://evil.example", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hdr := http.Header{}
			if tc.origin != "" {
				hdr.Set("Origin", tc.origin)
			}
			conn, resp, err := websocket.DefaultDialer.Dial(wsURL, hdr)
			if conn != nil {
				_ = conn.Close()
			}
			if resp == nil {
				t.Fatalf("no HTTP upgrade response: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want || (tc.want == http.StatusSwitchingProtocols && err != nil) {
				t.Fatalf("origin=%q status=%d want=%d err=%v", tc.origin, resp.StatusCode, tc.want, err)
			}
		})
	}
}
