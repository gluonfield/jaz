package browsercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type DesktopBackend struct {
	mu    sync.Mutex
	pages map[string]*browserPage
}

func NewDesktopBackend() *DesktopBackend {
	return &DesktopBackend{pages: map[string]*browserPage{}}
}

func (b *DesktopBackend) Connect(ctx context.Context, session string, ws *websocket.Conn) {
	page := &browserPage{conn: newCDPConn(ws)}
	b.mu.Lock()
	if b.pages[session] != nil {
		b.mu.Unlock()
		page.close()
		return
	}
	b.pages[session] = page
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pages, session)
		b.mu.Unlock()
		page.close()
	}()
	select {
	case <-ctx.Done():
	case <-page.conn.done:
	}
}

func (b *DesktopBackend) Call(ctx context.Context, input ActionInput) (ActionOutput, error) {
	b.mu.Lock()
	page := b.pages[input.Session]
	b.mu.Unlock()
	if page == nil {
		return ActionOutput{}, errors.New("open this conversation in the Jaz desktop app to connect its side browser")
	}
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return ActionOutput{}, err
	}
	if input.Action != ActionScript {
		if !page.opMu.TryLock() {
			return ActionOutput{}, errors.New("Another browser action is running for this conversation")
		}
		defer page.opMu.Unlock()
	}
	stopCancel := context.AfterFunc(ctx, func() { page.close() })
	defer stopCancel()
	switch input.Action {
	case ActionScript:
		var result extensionWireOutput
		if err := page.conn.call(ctx, "Jaz.run", map[string]any{"code": input.Text}, &result); err != nil {
			return ActionOutput{}, err
		}
		return actionOutput(result)
	case ActionStatus:
		return ActionOutput{Status: "ok", Text: "Jaz side browser is connected to this conversation."}, nil
	case ActionNavigate:
		url, err := normalizeBrowserURL(input.URL)
		if err != nil {
			return ActionOutput{}, err
		}
		if err := page.conn.call(ctx, "Jaz.open", map[string]any{"url": url}, nil); err != nil {
			return ActionOutput{}, err
		}
		page.contextID = 0
		if err := page.waitReady(ctx); err != nil {
			return ActionOutput{}, err
		}
		return page.semanticState(ctx)
	case ActionTabs:
		var tab BrowserTab
		if err := page.conn.call(ctx, "Jaz.tab", nil, &tab); err != nil {
			return ActionOutput{}, err
		}
		tabs := []BrowserTab{}
		if tab.ID != "" {
			tab.Ownership = "current_session"
			tabs = append(tabs, tab)
		}
		data, err := json.Marshal(TabsOutput{Tabs: tabs})
		return ActionOutput{Status: "ok", Data: data}, err
	case ActionClaimTab:
		return ActionOutput{}, errors.New("the side browser belongs to this conversation; use browser_navigate to open a page")
	}
	return page.call(ctx, input)
}

func (b *DesktopBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, page := range b.pages {
		page.close()
	}
	return nil
}
