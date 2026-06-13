package serverclient

import "testing"

func TestParseConnectionExtractsKey(t *testing.T) {
	conn, err := ParseConnection("https://jaz.example.com?key=secret")
	if err != nil {
		t.Fatal(err)
	}
	if conn.URL != "https://jaz.example.com" || conn.Key != "secret" {
		t.Fatalf("connection = %#v", conn)
	}
	t.Setenv("JAZ_API_KEY", "env-secret")
	conn, err = ParseConnection("https://jaz.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Key != "env-secret" {
		t.Fatalf("env key = %q", conn.Key)
	}
}
