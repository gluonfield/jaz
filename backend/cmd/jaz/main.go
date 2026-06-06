package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve", "server":
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

type sessionResponse struct {
	ID         string          `json:"id"`
	Slug       string          `json:"slug"`
	Runtime    string          `json:"runtime"`
	RuntimeRef *runtimeRefJSON `json:"runtime_ref,omitempty"`
}

type runtimeRefJSON struct {
	Agent string `json:"agent,omitempty"`
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
	fmt.Fprintln(os.Stderr, "usage: jaz serve [flags] | jaz server [flags] | jaz chat [flags]")
}
