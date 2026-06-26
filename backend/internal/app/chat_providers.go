package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/chatproviders/telegram"
	"github.com/wins/jaz/backend/internal/chatproviders/whatsapp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

func NewIntegrationRawWriter() integrationingest.RawWriter {
	return integrationingest.RawWriter{}
}

func NewWhatsAppChatProvider(lc fx.Lifecycle, layout runtimefiles.Layout, store *sqlitestore.Store, raw integrationingest.RawWriter) (ChatProviderOut, error) {
	provider, err := whatsapp.New(context.Background(), filepath.Join(layout.Connections, "whatsapp"), store, raw)
	if err != nil {
		return ChatProviderOut{}, err
	}
	lc.Append(fx.Hook{
		OnStart: provider.Start,
		OnStop: func(context.Context) error {
			return provider.Close()
		},
	})
	return chatProviderOut(provider), nil
}

func NewTelegramChatProvider(lc fx.Lifecycle, cfg Config, layout runtimefiles.Layout, store *sqlitestore.Store, raw integrationingest.RawWriter) (ChatProviderOut, error) {
	telegramConfig, ok, err := telegramProviderConfig(cfg)
	if err != nil {
		return ChatProviderOut{}, err
	}
	if !ok {
		return ChatProviderOut{}, nil
	}
	provider, err := telegram.New(filepath.Join(layout.Connections, "telegram"), telegramConfig, store, raw)
	if err != nil {
		return ChatProviderOut{}, err
	}
	lc.Append(fx.Hook{
		OnStart: provider.Start,
		OnStop: func(context.Context) error {
			return provider.Close()
		},
	})
	return chatProviderOut(provider), nil
}

func chatProviderOut(provider interface {
	connections.QRProvider
	connections.ChatSender
	connections.SessionDisconnecter
}) ChatProviderOut {
	return ChatProviderOut{
		QR:            []connections.QRProvider{provider},
		Senders:       []connections.ChatSender{provider},
		Disconnecters: []connections.SessionDisconnecter{provider},
	}
}

func telegramProviderConfig(cfg Config) (telegram.Config, bool, error) {
	apiID := cfg.Connections.Telegram.APIID
	apiHash := strings.TrimSpace(cfg.Connections.Telegram.APIHash)
	if apiID == 0 && apiHash == "" {
		return telegram.Config{}, false, nil
	}
	if apiID == 0 || apiHash == "" {
		return telegram.Config{}, false, fmt.Errorf("telegram api id and api hash must both be configured")
	}
	return telegram.Config{APIID: apiID, APIHash: apiHash}, true, nil
}
