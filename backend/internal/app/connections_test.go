package app

import (
	"strings"
	"testing"
)

func TestTelegramProviderConfigIsOptional(t *testing.T) {
	_, ok, err := telegramProviderConfig(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("telegram provider should be disabled when credentials are absent")
	}
}

func TestTelegramProviderConfigRejectsPartialCredentials(t *testing.T) {
	for _, cfg := range []Config{
		{Connections: ConnectionsConfig{Telegram: TelegramConnectionConfig{APIID: 123}}},
		{Connections: ConnectionsConfig{Telegram: TelegramConnectionConfig{APIHash: "hash"}}},
	} {
		_, _, err := telegramProviderConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "telegram api id and api hash") {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestTelegramProviderConfigTrimsHash(t *testing.T) {
	cfg, ok, err := telegramProviderConfig(Config{Connections: ConnectionsConfig{Telegram: TelegramConnectionConfig{
		APIID:   123,
		APIHash: " hash ",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg.APIID != 123 || cfg.APIHash != "hash" {
		t.Fatalf("config ok=%v cfg=%#v", ok, cfg)
	}
}
