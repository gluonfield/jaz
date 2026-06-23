package browserworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ExtensionProtocol = "jaz.browser.extension.v1"
	extensionTimeout  = 30 * time.Second
)

type ExtensionBridge struct {
	Fallback Backend
	Timeout  time.Duration

	seq    atomic.Uint64
	mu     sync.Mutex
	client *extensionClient
}

type ExtensionStatus struct {
	Connected     bool     `json:"connected"`
	ExtensionID   string   `json:"extension_id,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	Actions       []string `json:"actions,omitempty"`
	LastConnected string   `json:"last_connected_at,omitempty"`
}

type extensionClient struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan extensionResult
	done      chan struct{}

	mu     sync.Mutex
	status ExtensionStatus
}

type extensionHello struct {
	Type         string `json:"type"`
	Protocol     string `json:"protocol"`
	ExtensionID  string `json:"extension_id"`
	Capabilities struct {
		Actions []string `json:"actions"`
	} `json:"capabilities"`
}

type extensionCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Session  string `json:"session,omitempty"`
	Action   string `json:"action"`
	URL      string `json:"url,omitempty"`
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text,omitempty"`
	Key      string `json:"key,omitempty"`
	Amount   int    `json:"amount,omitempty"`
}

type extensionResult struct {
	ID     string              `json:"id"`
	Type   string              `json:"type"`
	OK     bool                `json:"ok"`
	Error  string              `json:"error,omitempty"`
	Output extensionWireOutput `json:"output,omitempty"`
}

type extensionWireOutput struct {
	Status          string `json:"status"`
	Text            string `json:"text,omitempty"`
	ImageBase64     string `json:"image_base64,omitempty"`
	ImageMIMEType   string `json:"image_mime_type,omitempty"`
	PDFBase64       string `json:"pdf_base64,omitempty"`
	PDFBase64Length int    `json:"pdf_base64_length,omitempty"`
}

var extensionUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		return origin == "" || strings.HasPrefix(origin, "chrome-extension://")
	},
}

func NewExtensionBridge(fallback Backend) *ExtensionBridge {
	return &ExtensionBridge{Fallback: fallback}
}

func (b *ExtensionBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	conn, err := extensionUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &extensionClient{
		conn:    conn,
		pending: map[string]chan extensionResult{},
		done:    make(chan struct{}),
	}
	b.setClient(client)
	client.readLoop(func() {
		b.clearClient(client)
	})
}

func (b *ExtensionBridge) Call(ctx context.Context, input ActionInput) (ActionOutput, error) {
	if client := b.currentClient(); client != nil {
		return client.call(ctx, b.nextID(), input, b.timeout())
	}
	if b.Fallback != nil {
		return b.Fallback.Call(ctx, input)
	}
	return ActionOutput{}, errors.New("browser extension bridge is not connected")
}

func (b *ExtensionBridge) Status() ExtensionStatus {
	client := b.currentClient()
	if client == nil {
		return ExtensionStatus{}
	}
	return client.statusSnapshot()
}

func (b *ExtensionBridge) Close() error {
	b.mu.Lock()
	client := b.client
	b.client = nil
	b.mu.Unlock()
	if client != nil {
		client.close()
	}
	if closer, ok := b.Fallback.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (b *ExtensionBridge) currentClient() *extensionClient {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.client
}

func (b *ExtensionBridge) setClient(client *extensionClient) {
	b.mu.Lock()
	old := b.client
	b.client = client
	b.mu.Unlock()
	if old != nil {
		old.close()
	}
}

func (b *ExtensionBridge) clearClient(client *extensionClient) {
	b.mu.Lock()
	if b.client == client {
		b.client = nil
	}
	b.mu.Unlock()
}

func (b *ExtensionBridge) nextID() string {
	return fmt.Sprintf("browser-%d", b.seq.Add(1))
}

func (b *ExtensionBridge) timeout() time.Duration {
	if b.Timeout > 0 {
		return b.Timeout
	}
	return extensionTimeout
}

func (c *extensionClient) call(ctx context.Context, id string, input ActionInput, timeout time.Duration) (ActionOutput, error) {
	resp := make(chan extensionResult, 1)
	c.addPending(id, resp)
	defer c.removePending(id)
	req := extensionCall{
		ID:       id,
		Type:     "call",
		Session:  input.Session,
		Action:   input.Action,
		URL:      input.URL,
		Selector: input.Selector,
		Text:     input.Text,
		Key:      input.Key,
		Amount:   input.Amount,
	}
	if err := c.write(req); err != nil {
		return ActionOutput{}, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case out := <-resp:
		if !out.OK {
			if strings.TrimSpace(out.Error) == "" {
				return ActionOutput{}, errors.New("browser extension call failed")
			}
			return ActionOutput{}, errors.New(out.Error)
		}
		return actionOutput(out.Output), nil
	case <-c.done:
		return ActionOutput{}, errors.New("browser extension disconnected")
	case <-timer.C:
		return ActionOutput{}, fmt.Errorf("browser extension timed out after %s", timeout)
	case <-ctx.Done():
		return ActionOutput{}, ctx.Err()
	}
}

func (c *extensionClient) readLoop(onDone func()) {
	defer onDone()
	defer c.close()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.handleMessage(data)
	}
}

func (c *extensionClient) handleMessage(data []byte) {
	var probe struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return
	}
	switch probe.Type {
	case "hello":
		var hello extensionHello
		if json.Unmarshal(data, &hello) == nil {
			c.setHello(hello)
		}
	case "result":
		var result extensionResult
		if json.Unmarshal(data, &result) == nil {
			c.resolve(result)
		}
	}
}

func (c *extensionClient) write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(value)
}

func (c *extensionClient) close() {
	select {
	case <-c.done:
	default:
		close(c.done)
		_ = c.conn.Close()
	}
}

func (c *extensionClient) addPending(id string, ch chan extensionResult) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.pending[id] = ch
}

func (c *extensionClient) removePending(id string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	delete(c.pending, id)
}

func (c *extensionClient) resolve(result extensionResult) {
	c.pendingMu.Lock()
	ch := c.pending[result.ID]
	c.pendingMu.Unlock()
	if ch != nil {
		ch <- result
	}
}

func (c *extensionClient) setHello(hello extensionHello) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = ExtensionStatus{
		Connected:     true,
		ExtensionID:   strings.TrimSpace(hello.ExtensionID),
		Protocol:      strings.TrimSpace(hello.Protocol),
		Actions:       append([]string(nil), hello.Capabilities.Actions...),
		LastConnected: time.Now().UTC().Format(time.RFC3339),
	}
}

func (c *extensionClient) statusSnapshot() ExtensionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.status
	out.Connected = true
	out.Actions = append([]string(nil), c.status.Actions...)
	return out
}

func actionOutput(out extensionWireOutput) ActionOutput {
	return ActionOutput{
		Status:          out.Status,
		Text:            out.Text,
		ImageBase64:     out.ImageBase64,
		ImageMIMEType:   out.ImageMIMEType,
		PDFBase64:       out.PDFBase64,
		PDFBase64Length: out.PDFBase64Length,
	}
}
