package main

import "testing"

func TestServeConnectURL(t *testing.T) {
	got := serveConnectURL(serveOptions{Addr: ":5299"}, "secret")
	if got != "http://127.0.0.1:5299?key=secret" {
		t.Fatalf("url = %q", got)
	}
	got = serveConnectURL(serveOptions{Addr: ":5299", PublicURL: "https://jaz.example.com/app"}, "secret")
	if got != "https://jaz.example.com?key=secret" {
		t.Fatalf("public url = %q", got)
	}
}

func TestParseServerConnectionExtractsKey(t *testing.T) {
	conn, err := parseServerConnection("https://jaz.example.com?key=secret")
	if err != nil {
		t.Fatal(err)
	}
	if conn.URL != "https://jaz.example.com" || conn.Key != "secret" {
		t.Fatalf("connection = %#v", conn)
	}
	t.Setenv("JAZ_API_KEY", "env-secret")
	conn, err = parseServerConnection("https://jaz.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if conn.Key != "env-secret" {
		t.Fatalf("env key = %q", conn.Key)
	}
}
