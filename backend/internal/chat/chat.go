package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	EventUserMessage      = "user_message"
	EventAssistantDelta   = "assistant_delta"
	EventAssistantMessage = "assistant_message"
	EventToolCall         = "tool_call"
	EventToolResult       = "tool_result"
	EventAsync            = "async"
	EventError            = "error"
	EventDone             = "done"
)

type Session struct {
	ID       string
	Slug     string
	Runtime  string
	ACPAgent string
}

type Event struct {
	Type      string                  `json:"type"`
	SessionID string                  `json:"session_id,omitempty"`
	Role      string                  `json:"role,omitempty"`
	Content   string                  `json:"content,omitempty"`
	Delta     string                  `json:"delta,omitempty"`
	Message   *provider.Message       `json:"message,omitempty"`
	ToolCall  *provider.ToolCall      `json:"tool_call,omitempty"`
	ToolName  string                  `json:"tool_name,omitempty"`
	Result    string                  `json:"result,omitempty"`
	Error     string                  `json:"error,omitempty"`
	ACP       *sessionevents.ACPEvent `json:"acp,omitempty"`
	At        time.Time               `json:"at,omitempty"`
}

type Result struct {
	Event Event
	Err   error
}

type Client struct {
	HTTP      *http.Client
	ServerURL string
}

func NewClient(httpClient *http.Client, serverURL string) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{HTTP: httpClient, ServerURL: strings.TrimRight(serverURL, "/")}
}

func (c Client) LoadHistory(ctx context.Context, sessionID string) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/sessions/%s/messages", c.ServerURL, sessionID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	var out messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(out.Messages)+len(out.Activity))
	for _, msg := range out.Messages {
		events = append(events, eventsFromMessage(sessionID, msg)...)
	}
	for _, activity := range out.Activity {
		if text := strings.TrimSpace(activity.Text); text != "" {
			events = append(events, Event{Type: EventAsync, SessionID: sessionID, Role: activity.Kind, Content: text, At: activity.At})
		}
	}
	return events, nil
}

func (c Client) Stream(ctx context.Context, sessionID, message string) <-chan Result {
	out := make(chan Result, 64)
	go c.stream(ctx, sessionID, message, out)
	return out
}

func (c Client) Subscribe(ctx context.Context, sessionID string) <-chan Result {
	out := make(chan Result, 16)
	go c.subscribe(ctx, sessionID, out)
	return out
}

type messagesResponse struct {
	Messages []provider.Message      `json:"messages"`
	Activity []storage.ActivityEntry `json:"activity"`
}

func eventsFromMessage(sessionID string, msg provider.Message) []Event {
	role := provider.MessageRole(msg)
	content := strings.TrimSpace(provider.MessageContent(msg))
	switch role {
	case "user":
		return []Event{{Type: EventUserMessage, SessionID: sessionID, Role: role, Content: content, Message: &msg}}
	case "assistant":
		events := make([]Event, 0, 1+len(provider.MessageToolCalls(msg)))
		if content != "" {
			events = append(events, Event{Type: EventAssistantMessage, SessionID: sessionID, Role: role, Content: content, Message: &msg})
		}
		for _, call := range provider.MessageToolCalls(msg) {
			events = append(events, Event{Type: EventToolCall, SessionID: sessionID, ToolCall: &call, ToolName: provider.ToolCallName(call)})
		}
		return events
	case "tool":
		return []Event{{Type: EventToolResult, SessionID: sessionID, Role: role, Content: content, Result: content, Message: &msg}}
	default:
		return nil
	}
}

func (c Client) stream(ctx context.Context, sessionID, message string, out chan<- Result) {
	defer close(out)
	user := provider.UserMessage(message)
	if !send(ctx, out, Result{Event: Event{Type: EventUserMessage, SessionID: sessionID, Role: "user", Content: message, Message: &user, At: time.Now().UTC()}}) {
		return
	}
	payload, _ := json.Marshal(map[string]string{"message": message})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/messages:stream", c.ServerURL, sessionID), bytes.NewReader(payload))
	if err != nil {
		send(ctx, out, Result{Err: err})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		send(ctx, out, Result{Err: err})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		send(ctx, out, Result{Err: fmt.Errorf("stream failed: %s", strings.TrimSpace(string(body)))})
		return
	}
	c.readStream(ctx, sessionID, resp.Body, out)
}

func (c Client) readStream(ctx context.Context, sessionID string, r io.Reader, out chan<- Result) {
	var assistant strings.Builder
	scanner := sseScanner(r)
	for scanner.Scan() {
		data := sseData(scanner.Text())
		if data == "" {
			continue
		}
		var event agent.StreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			send(ctx, out, Result{Err: err})
			return
		}
		switch event.Type {
		case agent.StreamDelta:
			assistant.WriteString(event.Delta)
			if !send(ctx, out, Result{Event: Event{Type: EventAssistantDelta, SessionID: sessionID, Role: "assistant", Content: event.Delta, Delta: event.Delta, At: event.At}}) {
				return
			}
		case agent.StreamToolCall:
			toolName := event.ToolName
			if event.ToolCall != nil {
				toolName = provider.ToolCallName(*event.ToolCall)
			}
			if !send(ctx, out, Result{Event: Event{Type: EventToolCall, SessionID: sessionID, ToolCall: event.ToolCall, ToolName: toolName, At: event.At}}) {
				return
			}
		case agent.StreamToolResult:
			if !send(ctx, out, Result{Event: Event{Type: EventToolResult, SessionID: sessionID, ToolName: event.ToolName, Content: event.Result, Result: event.Result, At: event.At}}) {
				return
			}
		case agent.StreamError:
			send(ctx, out, Result{Event: Event{Type: EventError, SessionID: sessionID, Error: event.Error, At: event.At}})
			return
		case agent.StreamDone:
			if content := assistant.String(); content != "" {
				msg := provider.AssistantMessage(content, nil)
				if !send(ctx, out, Result{Event: Event{Type: EventAssistantMessage, SessionID: sessionID, Role: "assistant", Content: content, Message: &msg, At: event.At}}) {
					return
				}
			}
			send(ctx, out, Result{Event: Event{Type: EventDone, SessionID: sessionID, At: event.At}})
			return
		}
	}
	if err := scanner.Err(); err != nil {
		send(ctx, out, Result{Err: err})
	}
}

func (c Client) subscribe(ctx context.Context, sessionID string, out chan<- Result) {
	defer close(out)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/sessions/%s/events", c.ServerURL, sessionID), nil)
	if err != nil {
		send(ctx, out, Result{Err: err})
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		send(ctx, out, Result{Err: err})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	scanner := sseScanner(resp.Body)
	for scanner.Scan() {
		data := sseData(scanner.Text())
		if data == "" {
			continue
		}
		var event sessionevents.Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			send(ctx, out, Result{Err: err})
			return
		}
		if strings.TrimSpace(event.Content) != "" && !send(ctx, out, Result{Event: Event{Type: EventAsync, SessionID: sessionID, Role: event.Type, Content: event.Content, ACP: event.ACP, At: event.At}}) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		send(ctx, out, Result{Err: err})
	}
}

func sseScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return scanner
}

func sseData(line string) string {
	if !strings.HasPrefix(line, "data:") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
}

func send(ctx context.Context, out chan<- Result, result Result) bool {
	select {
	case out <- result:
		return true
	case <-ctx.Done():
		return false
	}
}
