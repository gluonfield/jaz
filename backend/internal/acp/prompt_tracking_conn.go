package acp

import (
	"context"
	"sync"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

type promptTrackingConn struct {
	jsonrpc.MessageConn

	sendMu sync.Mutex
	cbMu   sync.RWMutex
	onSent func()
}

func newPromptTrackingConn(conn jsonrpc.MessageConn) *promptTrackingConn {
	return &promptTrackingConn{MessageConn: conn}
}

func (c *promptTrackingConn) setOnPromptSent(fn func()) {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.onSent = fn
}

func (c *promptTrackingConn) Send(ctx context.Context, msg *jsonrpc.Message) error {
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
