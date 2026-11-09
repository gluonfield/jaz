package desktopbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Conn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan reply
	closed   bool
	closeErr error
	done     chan struct{}
	onEvent  func(Message)
}

type Message struct {
	SessionID string          `json:"sessionId,omitempty"`
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    any             `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type reply struct {
	msg Message
	err error
}

func New(ws *websocket.Conn, readLimit int64, onEvent func(Message)) *Conn {
	ws.SetReadLimit(readLimit)
	conn := &Conn{
		ws:      ws,
		pending: map[int64]chan reply{},
		done:    make(chan struct{}),
		onEvent: onEvent,
	}
	KeepAlive(ws, conn.done)
	go conn.readLoop()
	return conn
}

func (c *Conn) Call(ctx context.Context, method string, params any, out any) error {
	return c.CallSession(ctx, "", method, params, out)
}

func (c *Conn) CallSession(ctx context.Context, sessionID, method string, params any, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, ch, err := c.reserve()
	if err != nil {
		return err
	}
	msg := Message{ID: id, Method: method, Params: params, SessionID: sessionID}
	c.writeMu.Lock()
	deadline := time.Now().Add(10 * time.Second)
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
	}
	_ = c.ws.SetWriteDeadline(deadline)
	err = c.ws.WriteJSON(msg)
	c.writeMu.Unlock()
	if err != nil {
		c.drop(id)
		return err
	}
	select {
	case reply := <-ch:
		if reply.err != nil {
			return reply.err
		}
		if reply.msg.Error != nil {
			return fmt.Errorf("desktop %s failed: %s", method, reply.msg.Error.Message)
		}
		if out != nil && len(reply.msg.Result) > 0 {
			if err := json.Unmarshal(reply.msg.Result, out); err != nil {
				return err
			}
		}
		return nil
	case <-ctx.Done():
		c.drop(id)
		return ctx.Err()
	}
}

func (c *Conn) reserve() (int64, chan reply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		if c.closeErr != nil {
			return 0, nil, c.closeErr
		}
		return 0, nil, errors.New("desktop connection is closed")
	}
	c.nextID++
	ch := make(chan reply, 1)
	c.pending[c.nextID] = ch
	return c.nextID, ch, nil
}

func (c *Conn) drop(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Conn) readLoop() {
	for {
		var msg Message
		if err := c.ws.ReadJSON(&msg); err != nil {
			c.fail(err)
			return
		}
		if msg.ID == 0 {
			if c.onEvent != nil {
				c.onEvent(msg)
			}
			continue
		}
		c.mu.Lock()
		ch := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- reply{msg: msg}
		}
	}
}

func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.done)
	c.closeErr = err
	pending := c.pending
	c.pending = map[int64]chan reply{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- reply{err: err}
	}
}

func (c *Conn) Close() error {
	c.fail(errors.New("desktop connection is closed"))
	return c.ws.Close()
}

func (c *Conn) Done() <-chan struct{} {
	return c.done
}

func KeepAlive(ws *websocket.Conn, done <-chan struct{}) {
	_ = ws.SetReadDeadline(time.Now().Add(75 * time.Second))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(75 * time.Second))
	})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
					_ = ws.Close()
					return
				}
			}
		}
	}()
}
