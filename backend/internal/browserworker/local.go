package browserworker

import (
	"context"
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
	mu       sync.Mutex
	browser  *localBrowser
	pages    map[string]*browserPage
}

type localBrowser struct {
	port string
	stop func()
}

type browserPage struct {
	target targetInfo
	conn   *cdpConn
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
	case "status":
		return b.status(ctx, input)
	case "tabs":
		return b.tabs(ctx)
	}
	page, err := b.page(ctx, input.Session)
	if err != nil {
		return ActionOutput{}, err
	}
	switch action {
	case "navigate":
		return page.navigate(ctx, input.URL)
	case "snapshot":
		return page.snapshot(ctx)
	case "state":
		return page.semanticState(ctx)
	case "screenshot":
		return page.screenshot(ctx)
	case "click":
		return page.click(ctx, input.Selector)
	case "hover":
		return page.hover(ctx, input.Selector)
	case "type":
		return page.typeText(ctx, input.Selector, input.Text)
	case "fill":
		return page.fill(ctx, input.Selector, input.Text)
	case "select":
		return page.selectOption(ctx, input.Selector, input.Text)
	case "press":
		return page.press(ctx, input.Key)
	case "scroll":
		return page.scroll(ctx, input.Selector, input.Text, input.Amount)
	case "wait":
		return page.wait(ctx, input.Selector, input.Text, input.Amount)
	case "pdf":
		return page.pdf(ctx)
	default:
		return ActionOutput{}, fmt.Errorf("unsupported browser action %q", input.Action)
	}
}

func (b *LocalBackend) Close() error {
	b.mu.Lock()
	pages := b.pages
	b.pages = map[string]*browserPage{}
	browser := b.browser
	b.browser = nil
	b.mu.Unlock()
	for _, page := range pages {
		page.close()
	}
	stopLocalBrowser(browser)
	return nil
}

func (b *LocalBackend) status(ctx context.Context, input ActionInput) (ActionOutput, error) {
	page, err := b.page(ctx, input.Session)
	if err != nil {
		return ActionOutput{}, err
	}
	info, err := page.info(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Browser connected.\n" + info}, nil
}

func (b *LocalBackend) tabs(ctx context.Context) (ActionOutput, error) {
	port, err := b.browserPort(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	targets, err := listTargets(ctx, b.httpClient(), port)
	if err != nil {
		return ActionOutput{}, err
	}
	var lines []string
	for _, target := range targets {
		if target.Type != "page" {
			continue
		}
		title := strings.TrimSpace(target.Title)
		if title == "" {
			title = "(untitled)"
		}
		lines = append(lines, fmt.Sprintf("- %s %s %s", target.ID, title, target.URL))
	}
	if len(lines) == 0 {
		lines = []string{"No page tabs are open."}
	}
	return ActionOutput{Status: "ok", Text: strings.Join(lines, "\n")}, nil
}

func (b *LocalBackend) page(ctx context.Context, session string) (*browserPage, error) {
	key := localPageKey(session)
	if page := b.cachedPage(key); page != nil {
		if page.ping(ctx) == nil {
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

func (b *LocalBackend) cachedPage(key string) *browserPage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pages[key]
}

func (b *LocalBackend) storePage(key string, page *browserPage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pages == nil {
		b.pages = map[string]*browserPage{}
	}
	if old := b.pages[key]; old != nil && old != page {
		old.close()
	}
	b.pages[key] = page
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
	for _, method := range []string{"Page.enable", "Runtime.enable", "Accessibility.enable"} {
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
	return fmt.Sprintf("title: %s\nurl: %s\nready: %s", info.Title, info.URL, info.Ready), nil
}
