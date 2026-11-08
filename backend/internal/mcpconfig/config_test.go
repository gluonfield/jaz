package mcpconfig

import (
	"reflect"
	"testing"
)

func TestValidateStdioPreservesExecutableArgumentsAndEnvironment(t *testing.T) {
	input, err := ValidateInput(ServerInput{
		Name: " Local ", Command: " /path with spaces/server ",
		Args: []string{"--mcp", "", " whitespace ", "$(literal); argument"},
		Env:  map[string]string{"TOKEN": " secret with spaces "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "Local" || input.Transport != TransportStdio || input.Command != "/path with spaces/server" || input.Env["TOKEN"] != " secret with spaces " || !reflect.DeepEqual(input.Args, []string{"--mcp", "", " whitespace ", "$(literal); argument"}) {
		t.Fatalf("stdio input changed = %#v", input)
	}
}

func TestValidateStdioRejectsInvalidOrMixedConfiguration(t *testing.T) {
	for _, input := range []ServerInput{
		{Name: "Local", Transport: TransportStdio},
		{Name: "Local", Transport: "unknown", URL: "https://example.test"},
		{Name: "Local", Command: "server", URL: "https://example.test"},
		{Name: "Local", Transport: TransportStreamableHTTP, Command: "server", URL: "https://example.test"},
		{Name: "Local", Command: "server", BearerToken: "token"},
		{Name: "Local", Command: "server", OAuth: OAuthConfig{ClientID: "client"}},
		{Name: "Local", Command: "server", Headers: []Header{{Name: "X-Key", Value: "value"}}},
		{Name: "Local", Command: "server\x00"},
		{Name: "Local", Command: "server", Args: []string{"arg\x00"}},
		{Name: "Local", Command: "server", Env: map[string]string{"INVALID-NAME": "value"}},
		{Name: "Local", Command: "server", Env: map[string]string{"KEY": "value\x00"}},
	} {
		if _, err := ValidateInput(input); err == nil {
			t.Fatalf("invalid configuration accepted: %#v", input)
		}
	}
}

func TestValidateInputNormalizesHeadersAndURL(t *testing.T) {
	input, err := ValidateInput(ServerInput{
		Name:              " Docs ",
		URL:               "HTTPS://mcp.example.com/mcp",
		Enabled:           true,
		BearerTokenEnvVar: " DOCS_TOKEN ",
		Headers: []Header{
			{Name: " X-Team ", Value: "platform"},
			{Name: " X-Secret ", EnvVar: " DOCS_SECRET "},
			{},
		},
		OAuth: OAuthConfig{
			ClientID:           " client-id ",
			ClientSecretEnvVar: " DOCS_OAUTH_SECRET ",
			Issuer:             " https://accounts.google.com/ ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "Docs" || input.URL != "https://mcp.example.com/mcp" ||
		input.BearerTokenEnvVar != "DOCS_TOKEN" {
		t.Fatalf("input = %#v", input)
	}
	if input.OAuth.ClientID != "client-id" ||
		input.OAuth.ClientSecretEnvVar != "DOCS_OAUTH_SECRET" ||
		input.OAuth.Issuer != "https://accounts.google.com" {
		t.Fatalf("oauth = %#v", input.OAuth)
	}
	if len(input.Headers) != 2 ||
		input.Headers[0].Name != "X-Team" || input.Headers[0].Value != "platform" ||
		input.Headers[1].Name != "X-Secret" || input.Headers[1].EnvVar != "DOCS_SECRET" {
		t.Fatalf("headers = %#v", input.Headers)
	}
}

func TestValidateInputRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		input ServerInput
	}{
		{
			name:  "unsupported scheme",
			input: ServerInput{Name: "Docs", URL: "ftp://mcp.example.com/mcp"},
		},
		{
			name:  "missing host",
			input: ServerInput{Name: "Docs", URL: "https:///mcp"},
		},
		{
			name:  "url credentials",
			input: ServerInput{Name: "Docs", URL: "https://user:pass@mcp.example.com/mcp"},
		},
		{
			name:  "bad bearer env",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", BearerTokenEnvVar: "BAD-NAME"},
		},
		{
			name:  "bad literal header",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", Headers: []Header{{Name: "Bad Header", Value: "x"}}},
		},
		{
			name:  "bad header env var",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", Headers: []Header{{Name: "X-Secret", EnvVar: "BAD-NAME"}}},
		},
		{
			name:  "ambiguous header value",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", Headers: []Header{{Name: "X-Secret", Value: "secret", EnvVar: "DOCS_SECRET"}}},
		},
		{
			name:  "oauth secret without client id",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", OAuth: OAuthConfig{ClientSecretEnvVar: "DOCS_OAUTH_SECRET"}},
		},
		{
			name:  "bad oauth secret env",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", OAuth: OAuthConfig{ClientID: "client", ClientSecretEnvVar: "BAD-NAME"}},
		},
		{
			name:  "bad oauth issuer",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", OAuth: OAuthConfig{Issuer: "http://accounts.google.com"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ValidateInput(tt.input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
