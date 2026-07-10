package integrations

import "testing"

func TestNormalizeAlias(t *testing.T) {
	tests := map[string]string{
		" Personal Gmail ":       "personal-gmail",
		"augustinas@example.com": "augustinas-example-com",
		"Work/Gmail":             "work-gmail",
		"---":                    "",
	}
	for input, want := range tests {
		if got := NormalizeAlias(input); got != want {
			t.Fatalf("NormalizeAlias(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefaultAliasPrefersEmailLocalPart(t *testing.T) {
	if got := DefaultAlias("augustinas@example.com", "provider-id"); got != "augustinas" {
		t.Fatalf("alias = %q, want augustinas", got)
	}
	if got := DefaultAlias("", "accounts/123"); got != "accounts-123" {
		t.Fatalf("alias = %q, want accounts-123", got)
	}
}

func TestSourceSlugKeepsReadablePrefixAndStableHash(t *testing.T) {
	got := SourceSlug("user:276369933")
	if got != "user-276369933-7be65a27" {
		t.Fatalf("source slug = %q", got)
	}
}

func TestPluginUsesConnectionMCP(t *testing.T) {
	cases := map[string]struct {
		plugin Plugin
		want   bool
	}{
		"no remote": {},
		"generic remote mcp": {
			plugin: Plugin{
				Auth:      []AuthOption{{Kind: AuthKindRemoteMCP}},
				RemoteMCP: &RemoteMCP{URL: "https://mcp.example.com"},
			},
		},
		"bearer token backed mcp": {
			plugin: Plugin{
				Auth:      []AuthOption{{Kind: AuthKindOAuth}},
				RemoteMCP: &RemoteMCP{URL: "https://mcp.example.com", TokenAuth: true},
			},
			want: true,
		},
		"mcp oauth backed connection": {
			plugin: Plugin{
				Auth:      []AuthOption{{Kind: AuthKindMCPConnection}},
				RemoteMCP: &RemoteMCP{URL: "https://mcp.example.com"},
			},
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.plugin.UsesConnectionMCP(); got != tc.want {
				t.Fatalf("UsesConnectionMCP() = %v, want %v", got, tc.want)
			}
		})
	}
}
