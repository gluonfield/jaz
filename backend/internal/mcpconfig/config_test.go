package mcpconfig

import "testing"

func TestValidateInputNormalizesHeadersAndURL(t *testing.T) {
	input, err := ValidateInput(ServerInput{
		Name:              " Docs ",
		URL:               "HTTPS://mcp.example.com/mcp",
		Enabled:           true,
		BearerTokenEnvVar: " DOCS_TOKEN ",
		Headers: []Header{
			{Name: " X-Team ", Value: "platform"},
			{},
		},
		EnvHeaders: []EnvHeader{
			{Name: " X-Secret ", EnvVar: " DOCS_SECRET "},
			{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "Docs" || input.URL != "https://mcp.example.com/mcp" ||
		input.BearerTokenEnvVar != "DOCS_TOKEN" {
		t.Fatalf("input = %#v", input)
	}
	if len(input.Headers) != 1 || input.Headers[0].Name != "X-Team" || input.Headers[0].Value != "platform" {
		t.Fatalf("headers = %#v", input.Headers)
	}
	if len(input.EnvHeaders) != 1 || input.EnvHeaders[0].Name != "X-Secret" || input.EnvHeaders[0].EnvVar != "DOCS_SECRET" {
		t.Fatalf("env headers = %#v", input.EnvHeaders)
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
			name:  "missing env header env var",
			input: ServerInput{Name: "Docs", URL: "https://mcp.example.com/mcp", EnvHeaders: []EnvHeader{{Name: "X-Secret"}}},
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
