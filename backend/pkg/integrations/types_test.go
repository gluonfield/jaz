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
