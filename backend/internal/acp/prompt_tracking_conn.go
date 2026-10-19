package acp

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

type promptTrackingConn struct {
	jsonrpc.MessageConn

	sendMu    sync.Mutex
	cbMu      sync.RWMutex
	onSending func() error
	onSent    func()
	// lastTraffic is when a message last crossed the connection either way,
	// in Unix nanoseconds.
	lastTraffic atomic.Int64
}

func newPromptTrackingConn(conn jsonrpc.MessageConn) *promptTrackingConn {
	c := &promptTrackingConn{MessageConn: conn}
	c.lastTraffic.Store(time.Now().UnixNano())
	return c
}

func (c *promptTrackingConn) setOnPromptSend(before func() error, after func()) {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.onSending, c.onSent = before, after
}

func (c *promptTrackingConn) Send(ctx context.Context, msg *jsonrpc.Message) error {
	c.lastTraffic.Store(time.Now().UnixNano())
	var before func() error
	var after func()
	if msg.IsRequest() && msg.Method == acpschema.AgentMethodSessionPrompt {
		var prompt acpschema.PromptRequest
		if json.Unmarshal(msg.Params, &prompt) == nil {
			if _, side := sideChatScopeFromMeta(prompt.Meta, ""); !side {
				c.cbMu.RLock()
				before, after = c.onSending, c.onSent
				c.cbMu.RUnlock()
			}
		}
	}
	if before != nil {
		if err := before(); err != nil {
			return err
		}
	}
	c.sendMu.Lock()
	err := c.MessageConn.Send(ctx, msg)
	c.sendMu.Unlock()
	if err == nil && after != nil {
		after()
	}
	return err
}

func (c *promptTrackingConn) Receive(ctx context.Context) (*jsonrpc.Message, error) {
	msg, err := c.MessageConn.Receive(ctx)
	if err == nil {
		c.lastTraffic.Store(time.Now().UnixNano())
	}
	return msg, err
}

func (c *promptTrackingConn) quietSince(t time.Time) bool {
	return c.lastTraffic.Load() < t.UnixNano()
}
