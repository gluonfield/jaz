package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/codexcompat"
	"github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/server"
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

func runServe(args []string) error {
	loaded, err := config.Load()
	if err != nil {
		return err
	}
	cfg := loaded.Jaz
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	fs.StringVar(&cfg.Root, "root", cfg.Root, "Jaz root directory")
	fs.StringVar(&cfg.Workspace, "workspace", cfg.Workspace, "default workspace")
	fs.StringVar(&cfg.Provider.Type, "provider", cfg.Provider.Type, "provider: openai, openrouter, or mock")
	fs.StringVar(&cfg.Provider.APIKey, "api-key", cfg.Provider.APIKey, "provider API key")
	fs.StringVar(&cfg.Provider.Model, "model", cfg.Provider.Model, "model name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	agentRuntime, store, err := app.BuildAgent(cfg)
	if err != nil {
		return err
	}
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = store.DefaultWorkspace()
	}
	srv := &http.Server{
		Addr:    *addr,
		Handler: (&server.Server{Agent: agentRuntime, Store: store, SystemPrompt: codexcompat.DefaultSystemPrompt}).Handler(),
	}
	fmt.Printf("jaz server listening on %s\n", displayAddr(*addr))
	fmt.Printf("root: %s\n", store.RootDir())
	fmt.Printf("workspace: %s\n", workspace)
	return srv.ListenAndServe()
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
	if *last {
		session, err := lastSession(client, *serverURL)
		if err != nil {
			return err
		}
		*sessionID = session.ID
	} else if *sessionID == "" {
		id, err := createSession(client, *serverURL)
		if err != nil {
			return err
		}
		*sessionID = id
	}
	fmt.Printf("session: %s\n", *sessionID)

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

func createSession(client *http.Client, serverURL string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(serverURL, "/")+"/v1/sessions", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create session failed: %s", strings.TrimSpace(string(body)))
	}
	var out sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: jaz serve [flags] | jaz chat [flags]")
}
