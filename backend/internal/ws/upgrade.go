package ws

import (
	"net/http"

	"github.com/gorilla/websocket"
)

const Protocol = "jaz.v1"

var Upgrader = websocket.Upgrader{
	Subprotocols: []string{Protocol},
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}
