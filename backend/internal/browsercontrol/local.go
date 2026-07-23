package browsercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBrowserSession = "default"
	defaultScrollAmount   = 800
	devToolsPortFile      = "DevToolsActivePort"
)

type LocalBackend struct {
	Root       string
	ChromePath string
	HTTPClient *http.Client

	launchMu sync.Mutex
	pageMu   sync.Mutex
	mu       sync.Mutex
	browser  *localBrowser
	pages    map[string]*browserPage
}

type localBrowser struct {
	port string
	stop func()
}

type browserPage struct {
	target    targetInfo
	conn      *cdpConn
	opMu      sync.Mutex
	contextID int64
}

type targetInfo struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	Title                string `json:"title"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func NewLocalBackend(root string) *LocalBackend {
	return &LocalBackend{Root: strings.TrimSpace(root), pages: map[string]*browserPage{}}
}

func (b *LocalBackend) Call(ctx context.Context, input ActionInput) (ActionOutput, error) {
	action := strings.ToLower(strings.TrimSpace(input.Action))
	input.Action = action
	switch action {
	case ActionStatus:
		return b.status(ctx, input)
	case ActionTabs:
		return b.tabs(ctx, input.Session)
	case ActionClaimTab:
		return b.claimTab(ctx, input.Session, input.TabID)
	case ActionNavigate, ActionState, ActionFind, ActionScreenshot, ActionClick,
		ActionFormInput, ActionPress, ActionScroll, ActionWait:
	default:
		return ActionOutput{}, UnsupportedActionError{Action: input.Action}
	}
	var page *browserPage
	var err error
	if action == ActionNavigate {
		if _, err := normalizeBrowserURL(input.URL); err != nil {
			return ActionOutput{}, err
		}
		page, err = b.page(ctx, input.Session)
	} else {
		page, err = b.existingPage(ctx, input.Session)
	}
	if err != nil {
		return ActionOutput{}, err
	}
	page.opMu.Lock()
	defer page.opMu.Unlock()
	if b.cachedPage(localPageKey(input.Session)) != page {
		return ActionOutput{}, errors.New("browser session tab changed while the action was waiting; read the page again")
	}
	switch action {
	case ActionNavigate:
		return page.navigate(ctx, input.URL)
	case ActionState:
		return page.semanticState(ctx)
	case ActionFind:
		return page.find(ctx, input.Text)
	case ActionScreenshot:
		return page.screenshot(ctx)
	case ActionClick:
		return page.click(ctx, input.Ref)
	case ActionFormInput:
		return page.formInput(ctx, input.Ref, input.Value)
	case ActionPress:
		return page.press(ctx, input.Ref, input.Key)
	case ActionScroll:
		return page.scroll(ctx, input.Ref, input.Text, input.Amount)
	case ActionWait:
		return page.wait(ctx, input.Ref, input.Text, input.Amount)
	}
	return ActionOutput{}, UnsupportedActionError{Action: input.Action}
}

func (b *LocalBackend) Close() error {
	b.mu.Lock()
	pages := b.pages
	b.pages = map[string]*browserPage{}
	browser := b.browser
	b.browser = nil
	b.mu.Unlock()
	for _, page := range pages {
		page.opMu.Lock()
		page.close()
		page.opMu.Unlock()
	}
	stopLocalBrowser(browser)
	return nil
}

func (b *LocalBackend) status(ctx context.Context, _ ActionInput) (ActionOutput, error) {
	if _, ok := b.liveBrowserPort(ctx); !ok {
		return ActionOutput{
			Status: "idle",
			Text:   "Managed browser is not running. Call browser_navigate to create an isolated session tab.",
		}, nil
	}
	return ActionOutput{Status: "ok", Text: "Managed browser is running."}, nil
}

func (b *LocalBackend) tabs(ctx context.Context, session string) (ActionOutput, error) {
	port, ok := b.liveBrowserPort(ctx)
	if !ok {
		data, err := json.Marshal(TabsOutput{Tabs: []BrowserTab{}})
		if err != nil {
			return ActionOutput{}, err
		}
		return ActionOutput{Status: "ok", Text: "No managed browser tabs are open.", Data: data}, nil
	}
	targets, err := listTargets(ctx, b.httpClient(), port)
	if err != nil {
		return ActionOutput{}, err
	}
	ownershipByTarget := b.ownershipByTarget(session)
	var tabs []BrowserTab
	for _, target := range targets {
		if target.Type != "page" {
			continue
		}
		title := strings.TrimSpace(target.Title)
		if title == "" {
			title = "(untitled)"
		}
		ownership := ownershipByTarget[target.ID]
		tabs = append(tabs, BrowserTab{ID: target.ID, Title: title, URL: target.URL, Ownership: ownership})
	}
	tabs = boundBrowserTabs(tabs)
	lines := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		lines = append(lines, fmt.Sprintf("- %s %s %s %s", tab.ID, tab.Ownership, tab.Title, tab.URL))
	}
	if len(lines) == 0 {
		lines = []string{"No page tabs are open."}
	}
	data, err := json.Marshal(TabsOutput{Tabs: tabs})
	if err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: strings.Join(lines, "\n"), Data: data}, nil
}

func (b *LocalBackend) claimTab(ctx context.Context, session, tabID string) (ActionOutput, error) {
	b.pageMu.Lock()
	defer b.pageMu.Unlock()
	port, ok := b.liveBrowserPort(ctx)
	if !ok {
		return ActionOutput{}, errors.New("managed browser is not running; no existing tab can be claimed")
	}
	targets, err := listTargets(ctx, b.httpClient(), port)
	if err != nil {
		return ActionOutput{}, err
	}
	for _, target := range targets {
		if target.Type != "page" || target.ID != tabID {
			continue
		}
		if b.claimedByOtherSession(session, tabID) {
			return ActionOutput{}, fmt.Errorf("browser tab %q is already claimed by another agent session", tabID)
		}
		page, err := newBrowserPage(ctx, target)
		if err != nil {
			return ActionOutput{}, err
		}
		info, err := page.info(ctx)
		if err != nil {
			page.close()
			return ActionOutput{}, err
		}
		b.storePage(localPageKey(session), page)
		return ActionOutput{Status: "ok", Text: "Claimed managed browser tab.\n" + info}, nil
	}
	return ActionOutput{}, fmt.Errorf("browser tab %q was not found", tabID)
}

func (b *LocalBackend) page(ctx context.Context, session string) (*browserPage, error) {
	b.pageMu.Lock()
	defer b.pageMu.Unlock()
	key := localPageKey(session)
	if page := b.cachedPage(key); page != nil {
		page.opMu.Lock()
		err := page.ping(ctx)
		page.opMu.Unlock()
		if err == nil {
			return page, nil
		}
		b.removePage(key, page)
		page.close()
	}
	port, err := b.browserPort(ctx)
	if err != nil {
		return nil, err
	}
	target, err := createTarget(ctx, b.httpClient(), port, "about:blank")
	if err != nil {
		return nil, err
	}
	page, err := newBrowserPage(ctx, target)
	if err != nil {
		return nil, err
	}
	b.storePage(key, page)
	return page, nil
}

func (b *LocalBackend) existingPage(ctx context.Context, session string) (*browserPage, error) {
	key := localPageKey(session)
	page := b.cachedPage(key)
	if page == nil {
		return nil, errors.New("browser session has no tab; call browser_navigate with a URL first")
	}
	page.opMu.Lock()
	err := page.ping(ctx)
	page.opMu.Unlock()
	if err == nil {
		return page, nil
	}
	b.removePage(key, page)
	page.close()
	return nil, errors.New("browser session tab is no longer available; call browser_navigate with a URL first")
}

func (b *LocalBackend) ownershipByTarget(session string) map[string]string {
	currentKey := localPageKey(session)
	ownership := map[string]string{}
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, page := range b.pages {
		if page == nil || strings.TrimSpace(page.target.ID) == "" {
			continue
		}
		value := "other_session"
		if key == currentKey {
			value = "current_session"
		}
		if ownership[page.target.ID] != "current_session" {
			ownership[page.target.ID] = value
		}
	}
	return ownership
}

func (b *LocalBackend) claimedByOtherSession(session, targetID string) bool {
	currentKey := localPageKey(session)
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, page := range b.pages {
		if key != currentKey && page != nil && page.target.ID == targetID {
			return true
		}
	}
	return false
}

func (b *LocalBackend) cachedPage(key string) *browserPage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pages[key]
}

func (b *LocalBackend) storePage(key string, page *browserPage) {
	var old *browserPage
	b.mu.Lock()
	if b.pages == nil {
		b.pages = map[string]*browserPage{}
	}
	old = b.pages[key]
	b.mu.Unlock()
	if old != nil && old != page {
		old.opMu.Lock()
		defer old.opMu.Unlock()
	}
	b.mu.Lock()
	b.pages[key] = page
	b.mu.Unlock()
	if old != nil && old != page {
		old.close()
	}
}

func (b *LocalBackend) removePage(key string, page *browserPage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pages[key] == page {
		delete(b.pages, key)
	}
}

func localPageKey(session string) string {
	key := strings.TrimSpace(session)
	if key == "" {
		return defaultBrowserSession
	}
	return key
}

func (b *LocalBackend) browserPort(ctx context.Context) (string, error) {
	if port, ok := b.liveBrowserPort(ctx); ok {
		return port, nil
	}
	b.launchMu.Lock()
	defer b.launchMu.Unlock()
	if port, ok := b.liveBrowserPort(ctx); ok {
		return port, nil
	}
	b.mu.Lock()
	old := b.browser
	b.browser = nil
	b.mu.Unlock()
	stopLocalBrowser(old)
	browser, err := b.launchBrowser(ctx)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.browser = browser
	b.mu.Unlock()
	return browser.port, nil
}

func (b *LocalBackend) liveBrowserPort(ctx context.Context) (string, bool) {
	b.mu.Lock()
	browser := b.browser
	b.mu.Unlock()
	if browser == nil || strings.TrimSpace(browser.port) == "" {
		return "", false
	}
	if _, err := browserVersion(ctx, b.httpClient(), browser.port); err != nil {
		return "", false
	}
	return browser.port, true
}

func (b *LocalBackend) httpClient() *http.Client {
	if b.HTTPClient != nil {
		return b.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func newBrowserPage(ctx context.Context, target targetInfo) (*browserPage, error) {
	if strings.TrimSpace(target.WebSocketDebuggerURL) == "" {
		return nil, errors.New("browser target did not expose a websocket debugger URL")
	}
	conn, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, err
	}
	page := &browserPage{target: target, conn: conn}
	for _, method := range []string{"Page.enable", "Runtime.enable"} {
		if err := conn.call(ctx, method, map[string]any{}, nil); err != nil {
			page.close()
			return nil, err
		}
	}
	return page, nil
}

func (p *browserPage) close() {
	if p != nil && p.conn != nil {
		_ = p.conn.Close()
	}
}

func (p *browserPage) ping(ctx context.Context) error {
	var href string
	return p.eval(ctx, "location.href", &href)
}

func (p *browserPage) info(ctx context.Context) (string, error) {
	var info struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Ready string `json:"ready"`
	}
	if err := p.eval(ctx, `({title: document.title, url: location.href, ready: document.readyState})`, &info); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"title: %s\nurl: %s\nready: %s",
		shortenText(info.Title, 500),
		shortenText(info.URL, 2000),
		shortenText(info.Ready, 50),
	), nil
}
