package httpapi

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func UpgradeWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	// Negotiate a public protocol; authentication subprotocols remain request-only.
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"jaz"},
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	return upgrader.Upgrade(w, r, nil)
}
