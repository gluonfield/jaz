package browserworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

type cdpConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan cdpReply
	closed   bool
	closeErr error
}

type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params any             `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpReply struct {
	msg cdpMessage
	err error
}

func dialCDP(ctx context.Context, endpoint string) (*cdpConn, error) {
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, err
	}
	conn := &cdpConn{
		ws:      ws,
		pending: map[int64]chan cdpReply{},
	}
	go conn.readLoop()
	return conn, nil
}

func (c *cdpConn) call(ctx context.Context, method string, params any, out any) error {
	id, ch, err := c.reserve()
	if err != nil {
		return err
	}
	msg := cdpMessage{ID: id, Method: method, Params: params}
	c.writeMu.Lock()
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
			return fmt.Errorf("cdp %s failed: %s", method, reply.msg.Error.Message)
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

func (c *cdpConn) reserve() (int64, chan cdpReply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		if c.closeErr != nil {
			return 0, nil, c.closeErr
		}
		return 0, nil, errors.New("cdp connection is closed")
	}
	c.nextID++
	ch := make(chan cdpReply, 1)
	c.pending[c.nextID] = ch
	return c.nextID, ch, nil
}

func (c *cdpConn) drop(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *cdpConn) readLoop() {
	for {
		var msg cdpMessage
		if err := c.ws.ReadJSON(&msg); err != nil {
			c.fail(err)
			return
		}
		if msg.ID == 0 {
			continue
		}
		c.mu.Lock()
		ch := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- cdpReply{msg: msg}
		}
	}
}

func (c *cdpConn) fail(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.closeErr = err
	pending := c.pending
	c.pending = map[int64]chan cdpReply{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- cdpReply{err: err}
	}
}

func (c *cdpConn) Close() error {
	c.fail(errors.New("cdp connection is closed"))
	return c.ws.Close()
}
