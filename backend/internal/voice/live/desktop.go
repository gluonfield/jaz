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
	ctx   context.Context
	conn  *desktopbridge.Conn
	calls map[string]*DesktopCall
}

type DesktopCall struct {
	relay  *DesktopRelay
	host   *voiceHost
	id     string
	events chan string
	ctx    context.Context
	cancel context.CancelFunc
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
	hostCtx, cancel := context.WithCancel(ctx)
	host := &voiceHost{ctx: hostCtx, calls: map[string]*DesktopCall{}}
	desktopbridge.KeepAlive(ws, hostCtx.Done())
	host.conn = desktopbridge.New(ws, 512*1024, func(message desktopbridge.Message) {
		if message.Method != "Claude.voiceEvent" {
			return
		}
		var event struct {
			ID   string `json:"id"`
			Data string `json:"data"`
		}
		if json.Unmarshal(message.Params, &event) != nil || len(event.Data) > 512*1024 {
			return
		}
		r.mu.Lock()
		call := host.calls[event.ID]
		r.mu.Unlock()
		if call != nil {
			select {
			case call.events <- event.Data:
			case <-call.ctx.Done():
			default:
				call.cancel()
			}
		}
	})
	r.host = host
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.host = nil
		cancel()
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
	if host == nil || host.ctx.Err() != nil {
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
	if r.host == nil || r.host.ctx.Err() != nil {
		return nil, ErrDesktopDisconnected
	}
	ctx, cancel := context.WithCancel(r.host.ctx)
	call := &DesktopCall{relay: r, host: r.host, id: uuid.NewString(), events: make(chan string, 16), ctx: ctx, cancel: cancel}
	r.host.calls[call.id] = call
	return call, nil
}

func (c *DesktopCall) Command(ctx context.Context, command ClaudeCommand) (json.RawMessage, error) {
	if err := c.ctx.Err(); err != nil {
		if c.host.ctx.Err() != nil {
			return nil, ErrDesktopDisconnected
		}
		return nil, err
	}
	command.ID = c.id
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	var result json.RawMessage
	err := c.host.conn.Call(ctx, "Claude.voice", command, &result)
	if err == nil {
		return result, nil
	}
	select {
	case <-c.host.conn.Done():
	case <-c.host.ctx.Done():
	default:
		return result, err
	}
	return nil, ErrDesktopDisconnected
}

func (c *DesktopCall) Events() <-chan string {
	return c.events
}

func (c *DesktopCall) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *DesktopCall) Close() {
	c.once.Do(func() {
		c.relay.mu.Lock()
		delete(c.host.calls, c.id)
		c.cancel()
		c.relay.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.host.conn.Call(ctx, "Claude.voice", ClaudeCommand{Type: "close", ID: c.id}, nil)
	})
}
