package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/mcpconfig"
)

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
