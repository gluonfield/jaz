package serverclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/serverconfig"
)

type Connection struct {
	URL string
	Key string
}

func ParseConnection(raw string) (Connection, error) {
	u, err := url.Parse(serverconfig.DisplayAddr(raw))
	if err != nil {
		return Connection{}, err
	}
	key := strings.TrimSpace(u.Query().Get("key"))
	u.RawQuery = ""
	u.Fragment = ""
	if key == "" {
		key = strings.TrimSpace(os.Getenv("JAZ_API_KEY"))
	}
	return Connection{URL: strings.TrimRight(u.String(), "/"), Key: key}, nil
}

func HTTPClient(key string) *http.Client {
	if strings.TrimSpace(key) == "" {
		return http.DefaultClient
	}
	return &http.Client{Transport: authTransport{key: strings.TrimSpace(key), base: http.DefaultTransport}}
}

type Session struct {
	ID         string      `json:"id"`
	Slug       string      `json:"slug"`
	Runtime    string      `json:"runtime"`
	RuntimeRef *RuntimeRef `json:"runtime_ref,omitempty"`
}

type RuntimeRef struct {
	Agent string `json:"agent,omitempty"`
}

func CreateSession(client *http.Client, serverURL string) (Session, error) {
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(serverURL, "/")+"/v1/sessions", nil)
	if err != nil {
		return Session{}, err
	}
	return doSession(client, req, "create session")
}

func LastSession(client *http.Client, serverURL string) (Session, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(serverURL, "/")+"/v1/sessions?last=true", nil)
	if err != nil {
		return Session{}, err
	}
	return doSession(client, req, "last session")
}

func GetSession(client *http.Client, serverURL, sessionID string) (Session, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/sessions/%s", strings.TrimRight(serverURL, "/"), sessionID), nil)
	if err != nil {
		return Session{}, err
	}
	return doSession(client, req, "load session")
}

type authTransport struct {
	key  string
	base http.RoundTripper
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Header.Get("Authorization") != "" {
		return base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.key)
	return base.RoundTrip(clone)
}

func doSession(client *http.Client, req *http.Request, action string) (Session, error) {
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return Session{}, fmt.Errorf("%s failed: %s", action, strings.TrimSpace(string(body)))
	}
	var out Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Session{}, err
	}
	return out, nil
}
