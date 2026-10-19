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

	sendMu     sync.Mutex
	cbMu       sync.RWMutex
	onSending  func(id string)
	onSent     func()
	onAnswered func(id string)
	// lastTraffic is when a message last crossed the connection either way,
	// in Unix nanoseconds.
	lastTraffic atomic.Int64
}

func newPromptTrackingConn(conn jsonrpc.MessageConn) *promptTrackingConn {
	c := &promptTrackingConn{MessageConn: conn}
	c.lastTraffic.Store(time.Now().UnixNano())
	return c
}

// trackPrompts calls onSending with the id of each turn's session/prompt
// request before it goes out and onSent once it went out, and onAnswered with
// the id of each response as the connection reads it, before it reads on.
func (c *promptTrackingConn) trackPrompts(onSending func(id string), onSent func(), onAnswered func(id string)) {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.onSending, c.onSent, c.onAnswered = onSending, onSent, onAnswered
}

func (c *promptTrackingConn) Send(ctx context.Context, msg *jsonrpc.Message) error {
	c.lastTraffic.Store(time.Now().UnixNano())
	id := promptID(msg)
	c.cbMu.RLock()
	onSending, onSent := c.onSending, c.onSent
	c.cbMu.RUnlock()
	if id != "" && onSending != nil {
		onSending(id)
	}
	c.sendMu.Lock()
	err := c.MessageConn.Send(ctx, msg)
	c.sendMu.Unlock()
	if err == nil && id != "" && onSent != nil {
		onSent()
	}
	return err
}

func (c *promptTrackingConn) Receive(ctx context.Context) (*jsonrpc.Message, error) {
	msg, err := c.MessageConn.Receive(ctx)
	if err != nil {
		return msg, err
	}
	c.lastTraffic.Store(time.Now().UnixNano())
	c.cbMu.RLock()
	onAnswered := c.onAnswered
	c.cbMu.RUnlock()
	if msg.IsResponse() && onAnswered != nil {
		id, _ := jsonrpc.IDKey(*msg.ID)
		onAnswered(id)
	}
	return msg, nil
}

// promptID is the id of msg when it is a turn's session/prompt request; a
// side chat's prompt is not one.
func promptID(msg *jsonrpc.Message) string {
	if !msg.IsRequest() || msg.Method != acpschema.AgentMethodSessionPrompt {
		return ""
	}
	var prompt acpschema.PromptRequest
	if json.Unmarshal(msg.Params, &prompt) != nil {
		return ""
	}
	if _, side := sideChatScopeFromMeta(prompt.Meta, ""); side {
		return ""
	}
	id, _ := jsonrpc.IDKey(*msg.ID)
	return id
}

func (c *promptTrackingConn) quietSince(t time.Time) bool {
	return c.lastTraffic.Load() < t.UnixNano()
}
