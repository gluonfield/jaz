package browsercontrol

import (
	"context"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

func (b *DesktopBackend) ConnectHost(ctx context.Context, ws *websocket.Conn) {
	host := newCDPConn(ws)
	b.mu.Lock()
	if b.host != nil {
		b.mu.Unlock()
		_ = host.Close()
		return
	}
	b.host = host
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.host = nil
		b.mu.Unlock()
		_ = host.Close()
	}()
	select {
	case <-ctx.Done():
	case <-host.done:
	}
}

func (b *DesktopBackend) page(ctx context.Context, session string) (*browserPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	page, host := b.pages[session], b.host
	b.mu.Unlock()
	if page != nil {
		return page, nil
	}
	if host == nil {
		return nil, errors.New("Jaz desktop side browser is not connected; open or reconnect the Jaz desktop app")
	}
	if err := host.call(ctx, "Jaz.connect", map[string]string{"session": session}, nil); err != nil {
		return nil, err
	}
	for {
		b.mu.Lock()
		page, changed := b.pages[session], b.changed
		b.mu.Unlock()
		if page != nil {
			return page, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-host.done:
			return nil, errors.New("Jaz desktop side browser disconnected")
		case <-changed:
		}
	}
}
