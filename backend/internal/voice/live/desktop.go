package live

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/desktopbridge"
)

var ErrDesktopDisconnected = errors.New("open Jaz Desktop and keep it connected to use Claude voice")

type DesktopRelay struct {
	mu   sync.Mutex
	host *voiceHost
}

type voiceHost struct {
	conn  *desktopbridge.Conn
	calls map[string]*DesktopCall
}

type DesktopCall struct {
	relay  *DesktopRelay
	host   *voiceHost
	id     string
	events chan string
	done   chan struct{}
	once   sync.Once
}

func NewDesktopRelay() *DesktopRelay {
	return &DesktopRelay{}
}

func (r *DesktopRelay) Connect(ctx context.Context, ws *websocket.Conn) {
	r.mu.Lock()
	if r.host != nil {
		r.mu.Unlock()
		_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Another Jaz Desktop already provides Claude voice. Disconnect it before taking over."), time.Now().Add(5*time.Second))
		_ = ws.Close()
		return
	}
	host := &voiceHost{calls: map[string]*DesktopCall{}}
	host.conn = desktopbridge.New(ws, 512*1024, func(message desktopbridge.Message) {
		if message.Method != "Claude.voiceEvent" {
			return
		}
		data, err := json.Marshal(message.Params)
		if err != nil {
			return
		}
		var event struct {
			ID   string `json:"id"`
			Data string `json:"data"`
		}
		if json.Unmarshal(data, &event) != nil || len(event.Data) > 512*1024 {
			return
		}
		r.mu.Lock()
		call := host.calls[event.ID]
		r.mu.Unlock()
		if call != nil {
			select {
			case call.events <- event.Data:
			case <-call.done:
			default:
				call.stop()
			}
		}
	})
	r.host = host
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.host = nil
		for _, call := range host.calls {
			call.stop()
		}
		r.mu.Unlock()
		_ = host.conn.Close()
	}()
	select {
	case <-ctx.Done():
	case <-host.conn.Done():
	}
}

func (r *DesktopRelay) Status(ctx context.Context) ClaudeStatus {
	r.mu.Lock()
	host := r.host
	r.mu.Unlock()
	if host == nil {
		return ClaudeStatus{Reason: ErrDesktopDisconnected.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var result ClaudeStatus
	if err := host.conn.Call(ctx, "Claude.voice", ClaudeCommand{Type: "status"}, &result); err != nil {
		return ClaudeStatus{Reason: err.Error()}
	}
	return result
}

func (r *DesktopRelay) Open() (*DesktopCall, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.host == nil {
		return nil, ErrDesktopDisconnected
	}
	call := &DesktopCall{relay: r, host: r.host, id: uuid.NewString(), events: make(chan string, 16), done: make(chan struct{})}
	r.host.calls[call.id] = call
	return call, nil
}

func (c *DesktopCall) Command(ctx context.Context, command ClaudeCommand) (json.RawMessage, error) {
	command.ID = c.id
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	var result json.RawMessage
	err := c.host.conn.Call(ctx, "Claude.voice", command, &result)
	if err != nil {
		select {
		case <-c.host.conn.Done():
			return nil, ErrDesktopDisconnected
		default:
		}
	}
	return result, err
}

func (c *DesktopCall) Events() <-chan string {
	return c.events
}

func (c *DesktopCall) Done() <-chan struct{} {
	return c.done
}

func (c *DesktopCall) stop() {
	c.once.Do(func() { close(c.done) })
}

func (c *DesktopCall) Close() {
	c.stop()
	c.relay.mu.Lock()
	delete(c.host.calls, c.id)
	c.relay.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = c.Command(ctx, ClaudeCommand{Type: "close"})
}
