package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
			os.Exit(1)
		}
	case "chat":
		if err := runChat(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "chat:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runChat(args []string) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	serverURL := fs.String("server", "http://127.0.0.1:8080", "Jaz server URL")
	sessionID := fs.String("session", "", "existing session ID")
	last := fs.Bool("last", false, "connect to the last root session")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *last && *sessionID != "" {
		return fmt.Errorf("use either --last or --session, not both")
	}

	client := http.DefaultClient
	var session sessionResponse
	if *last {
		var err error
		session, err = lastSession(client, *serverURL)
		if err != nil {
			return err
		}
	} else if *sessionID == "" {
		var err error
		session, err = createSession(client, *serverURL)
		if err != nil {
			return err
		}
	} else {
		var err error
		session, err = getSession(client, *serverURL, *sessionID)
		if err != nil {
			return err
		}
	}
	*sessionID = session.ID
	fmt.Printf("session: %s\n", sessionLabel(session))
	_ = printHistory(client, *serverURL, *sessionID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go subscribeEvents(ctx, client, *serverURL, *sessionID)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		msg := strings.TrimSpace(scanner.Text())
		if msg == "" {
			continue
		}
		if msg == "/quit" || msg == "/exit" {
			break
		}
		if err := streamMessage(client, *serverURL, *sessionID, msg); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		fmt.Println()
	}
	return scanner.Err()
}

type sessionResponse struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

func createSession(client *http.Client, serverURL string) (sessionResponse, error) {
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(serverURL, "/")+"/v1/sessions", nil)
	if err != nil {
		return sessionResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return sessionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return sessionResponse{}, fmt.Errorf("create session failed: %s", strings.TrimSpace(string(body)))
	}
	var out sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return sessionResponse{}, err
	}
	return out, nil
}

func lastSession(client *http.Client, serverURL string) (sessionResponse, error) {
	endpoint := strings.TrimRight(serverURL, "/") + "/v1/sessions?last=true"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return sessionResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return sessionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return sessionResponse{}, fmt.Errorf("last session failed: %s", strings.TrimSpace(string(body)))
	}
	var out sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return sessionResponse{}, err
	}
	return out, nil
}

func getSession(client *http.Client, serverURL, sessionID string) (sessionResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/sessions/%s", strings.TrimRight(serverURL, "/"), sessionID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return sessionResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return sessionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return sessionResponse{}, fmt.Errorf("load session failed: %s", strings.TrimSpace(string(body)))
	}
	var out sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return sessionResponse{}, err
	}
	return out, nil
}

type messagesResponse struct {
	Messages []provider.Message      `json:"messages"`
	Activity []storage.ActivityEntry `json:"activity"`
}

func printHistory(client *http.Client, serverURL, sessionID string) error {
	endpoint := fmt.Sprintf("%s/v1/sessions/%s/messages", strings.TrimRight(serverURL, "/"), sessionID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var out messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	printed := false
	for _, msg := range out.Messages {
		role := provider.MessageRole(msg)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(provider.MessageContent(msg))
		if content == "" {
			continue
		}
		if !printed {
			fmt.Println("history:")
			printed = true
		}
		fmt.Printf("[%s] %s\n", role, content)
	}
	for _, entry := range out.Activity {
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}
		if !printed {
			fmt.Println("history:")
			printed = true
		}
		label := entry.Kind
		if entry.Status != "" {
			label += " " + entry.Status
		}
		fmt.Printf("[%s] %s\n", label, text)
	}
	return nil
}

func subscribeEvents(ctx context.Context, client *http.Client, serverURL, sessionID string) {
	endpoint := fmt.Sprintf("%s/v1/sessions/%s/events", strings.TrimRight(serverURL, "/"), sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var event sessionevents.Event
		if err := json.Unmarshal([]byte(data), &event); err != nil || strings.TrimSpace(event.Content) == "" {
			continue
		}
		fmt.Printf("\n[async]\n%s\n> ", strings.TrimSpace(event.Content))
	}
}

func streamMessage(client *http.Client, serverURL, sessionID, message string) error {
	payload, _ := json.Marshal(map[string]string{"message": message})
	endpoint := fmt.Sprintf("%s/v1/sessions/%s/messages:stream", strings.TrimRight(serverURL, "/"), sessionID)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream failed: %s", strings.TrimSpace(string(body)))
	}
	return readSSE(resp.Body)
}

func readSSE(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var event agent.StreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return err
		}
		switch event.Type {
		case agent.StreamDelta:
			fmt.Print(event.Delta)
		case agent.StreamToolCall:
			if event.ToolCall != nil {
				fmt.Printf("\n[tool] %s\n", provider.ToolCallName(*event.ToolCall))
			} else if event.ToolName != "" {
				fmt.Printf("\n[tool] %s\n", event.ToolName)
			}
		case agent.StreamToolResult:
			fmt.Printf("[tool result] %s\n", compact(event.Result, 600))
		case agent.StreamError:
			fmt.Printf("\n[error] %s\n", event.Error)
		case agent.StreamDone:
			return nil
		}
	}
	return scanner.Err()
}

func compact(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

func sessionLabel(session sessionResponse) string {
	if session.Slug == "" || session.Slug == session.ID {
		return session.ID
	}
	return session.Slug + " (" + session.ID + ")"
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: jaz serve [flags] | jaz chat [flags]")
}
