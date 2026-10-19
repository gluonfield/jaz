package acp

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

type promptTrackingConn struct {
	jsonrpc.MessageConn

	sendMu sync.Mutex
	cbMu   sync.RWMutex
	onSent func()
	// lastTraffic is when a message last crossed the connection either way,
	// in Unix nanoseconds.
	lastTraffic atomic.Int64
}

func newPromptTrackingConn(conn jsonrpc.MessageConn) *promptTrackingConn {
	c := &promptTrackingConn{MessageConn: conn}
	c.lastTraffic.Store(time.Now().UnixNano())
	return c
}

func (c *promptTrackingConn) setOnPromptSent(fn func()) {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.onSent = fn
}

func (c *promptTrackingConn) Send(ctx context.Context, msg *jsonrpc.Message) error {
	c.lastTraffic.Store(time.Now().UnixNano())
	c.sendMu.Lock()
	err := c.MessageConn.Send(ctx, msg)
	c.sendMu.Unlock()
	if err == nil && msg.IsRequest() && msg.Method == acpschema.AgentMethodSessionPrompt {
		c.cbMu.RLock()
		onSent := c.onSent
		c.cbMu.RUnlock()
		if onSent != nil {
			onSent()
		}
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
