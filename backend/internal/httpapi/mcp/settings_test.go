package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/mcpconfig"
)

func TestStdioSettingsMaskAndPreserveEnvironment(t *testing.T) {
	current := mcpconfig.Server{Transport: mcpconfig.TransportStdio, Command: "/bin/server", Args: []string{"--mcp"}, Env: map[string]string{"TOKEN": "secret-value"}}
	view := NewServerView(current, mcpconfig.ServerStatus{})
	data, err := json.Marshal(view)
	if err != nil || strings.Contains(string(data), "secret-value") || !strings.Contains(string(data), `"TOKEN":null`) {
		t.Fatalf("environment was not masked: %s, %v", data, err)
	}
	for _, body := range []string{
		`{"name":"Local","command":"/bin/server","args":["--mcp"],"env":{"TOKEN":null,"NEW":"new"}}`,
		`{"name":"Local","command":"/bin/server","args":["--mcp"],"env":{}}`,
	} {
		input, err := DecodeServerInput(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)), &current)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body, `"TOKEN"`) && (input.Env["TOKEN"] != "secret-value" || input.Env["NEW"] != "new") {
			t.Fatalf("saved environment was not preserved: %#v", input.Env)
		}
		if !strings.Contains(body, `"TOKEN"`) && len(input.Env) != 0 {
			t.Fatal("removed environment was retained")
		}
	}
	for _, body := range []string{
		`{"name":"Local","command":"/bin/different","args":["--mcp"],"env":{"TOKEN":null}}`,
		`{"name":"Local","command":"/bin/server","args":["--different"],"env":{"TOKEN":null}}`,
	} {
		if _, err := DecodeServerInput(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)), &current); err == nil {
			t.Fatal("saved environment was forwarded to a different command")
		}
	}
}

func TestChangingEndpointDoesNotForwardSavedCredentials(t *testing.T) {
	current := mcpconfig.Server{URL: "https://original.example/mcp", BearerToken: "original-secret", Headers: []mcpconfig.Header{{Name: "X-Key", Value: "original-header"}}}
	request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"name":"Moved tools","url":"https://different.example/mcp"}`))
	input, err := DecodeServerInput(request, &current)
	if err != nil {
		t.Fatal(err)
	}
	if input.BearerToken != "" || len(input.Headers) != 0 {
		t.Fatal("changing endpoints forwarded saved credentials to another server")
	}
}
