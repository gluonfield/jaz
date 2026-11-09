package browsercontrol

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/desktopbridge"
)

type cdpMessage = desktopbridge.Message

type cdpConn struct {
	*desktopbridge.Conn
	done <-chan struct{}
}

func newCDPConn(ws *websocket.Conn) *cdpConn {
	conn := desktopbridge.New(ws, browserWireReadLimit, nil)
	return &cdpConn{Conn: conn, done: conn.Done()}
}

func (c *cdpConn) call(ctx context.Context, method string, params, out any) error {
	return c.Call(ctx, method, params, out)
}

func (c *cdpConn) callSession(ctx context.Context, session, method string, params, out any) error {
	return c.CallSession(ctx, session, method, params, out)
}
