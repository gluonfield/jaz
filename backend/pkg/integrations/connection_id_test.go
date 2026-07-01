package integrations

import (
	"strings"
	"testing"
)

func TestConnectionID(t *testing.T) {
	first, err := ConnectionID("gmail", " Augustinas.Example@gmail.com ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ConnectionID("gmail", "augustinas-example@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("connection ids should include a hash suffix: %q", first)
	}
	if !strings.HasPrefix(first, "gmail:augustinas-example-gmail-com-") {
		t.Fatalf("connection id = %q", first)
	}
	if _, err := ConnectionID("slack", " "); err == nil {
		t.Fatal("expected empty account error")
	}
}
