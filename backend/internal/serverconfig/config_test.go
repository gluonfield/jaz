package serverconfig

import "testing"

func TestClientURL(t *testing.T) {
	got := ClientURL(Config{Addr: ":5299"}, "secret")
	if got != "http://127.0.0.1:5299?key=secret" {
		t.Fatalf("url = %q", got)
	}
	got = ClientURL(Config{Addr: ":5299", PublicURL: "https://jaz.example.com/app"}, "secret")
	if got != "https://jaz.example.com?key=secret" {
		t.Fatalf("public url = %q", got)
	}
}
