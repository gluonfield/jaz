package browsercontrol

import (
	"strings"
	"testing"
)

func TestSemanticPageStateRedactsPasswordAndSensitiveControls(t *testing.T) {
	state := PageState{
		URL:   "https://checkout.example.com",
		Title: "Checkout",
		Elements: []StateElement{
			{Ref: "e1", Tag: "input", InputType: "email", Name: "Email", Value: "user@example.com"},
			{Ref: "e2", Tag: "input", InputType: "password", Name: "Password", Value: "secret-pass", Text: "secret-pass"},
			{Ref: "e3", Tag: "input", InputType: "text", Name: "Card number", Value: "4242424242424242", Sensitive: true},
			{Ref: "e4", Tag: "input", InputType: "text", Name: "Security code", Text: "987", Sensitive: true},
		},
	}
	rendered := formatPageState(state)
	if !strings.Contains(rendered, "user@example.com") {
		t.Fatalf("expected non-sensitive email preserved, got:\n%s", rendered)
	}
	for _, secret := range []string{"secret-pass", "4242424242424242", "987"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("semantic page state leaked secret %q:\n%s", secret, rendered)
		}
	}
}

func TestAXTreeRedactsSensitiveAttributesAndVaultFilledNodes(t *testing.T) {
	frame := axFrame{cdpFrame: cdpFrame{ID: "main", LoaderID: "doc-1", URL: "https://checkout.example.com"}}
	nodes := []axNode{
		{ID: "root", Role: axValue{Value: "RootWebArea"}, ChildIDs: []string{"normal", "cc", "filled"}},
		{
			ID:        "normal",
			ParentID:  "root",
			Role:      axValue{Value: "textbox"},
			Name:      axValue{Value: "Order note"},
			Value:     axValue{Value: "Leave at porch"},
			BackendID: 10,
		},
		{
			ID:        "cc",
			ParentID:  "root",
			Role:      axValue{Value: "textbox"},
			Name:      axValue{Value: "Card number"},
			Value:     axValue{Value: "4242424242424242"},
			BackendID: 11,
			Properties: []axProperty{
				{Name: "autocomplete", Value: axValue{Value: "cc-number"}},
			},
		},
		{
			ID:        "filled",
			ParentID:  "root",
			Role:      axValue{Value: "textbox"},
			Name:      axValue{Value: "2FA Code"},
			Value:     axValue{Value: "654321"},
			BackendID: 12,
			Properties: []axProperty{
				{Name: "valuetext", Value: axValue{Value: "654321"}},
			},
		},
	}
	redacted := map[string]map[int64]bool{
		"main": {12: true},
	}
	state := axState{indices: map[string]int{}, targets: map[string]axTarget{}}
	state.appendTree(&axTree{Frame: frame, Nodes: nodes}, nil, redacted, 0, -1)
	rendered := renderAXRevision(nil, state.lines, true)

	if !strings.Contains(rendered, `Value: "Leave at porch"`) {
		t.Fatalf("expected non-sensitive textbox value preserved:\n%s", rendered)
	}
	if strings.Contains(rendered, "4242424242424242") {
		t.Fatalf("AX tree leaked cc-number value:\n%s", rendered)
	}
	if strings.Contains(rendered, "654321") {
		t.Fatalf("AX tree leaked vault-filled node value:\n%s", rendered)
	}
	if strings.Count(rendered, "[REDACTED]") < 2 {
		t.Fatalf("expected [REDACTED] markers in AX output:\n%s", rendered)
	}
}

func TestPaymentIframeOriginAllowlist(t *testing.T) {
	if !isPaymentIframeOrigin("https://js.stripe.com") {
		t.Fatal("expected https://js.stripe.com allowlisted")
	}
	if !isPaymentIframeOrigin("https://elements.braintreegateway.com") {
		t.Fatal("expected Braintree elements allowlisted")
	}
	for _, blocked := range []string{
		"http://js.stripe.com",
		"https://js.stripe.com.evil.com",
		"https://evil.example.com",
		"https://js.stripe.com:8443",
	} {
		if isPaymentIframeOrigin(blocked) {
			t.Fatalf("expected %q rejected from payment iframe allowlist", blocked)
		}
	}
}
