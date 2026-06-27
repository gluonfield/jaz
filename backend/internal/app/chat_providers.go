package app

import (
	"context"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/chatproviders/telegram"
	"github.com/wins/jaz/backend/internal/chatproviders/whatsapp"
	"github.com/wins/jaz/backend/internal/connections"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

func NewWhatsAppChatProvider(lc fx.Lifecycle, layout runtimefiles.Layout, store *sqlitestore.Store, raw integrationingest.RawWriter, logger *log.Logger) (ChatProviderOut, error) {
	provider, err := whatsapp.New(context.Background(), filepath.Join(layout.Connections, "whatsapp"), store, raw, logger)
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

func NewTelegramChatProvider(lc fx.Lifecycle, layout runtimefiles.Layout, store *sqlitestore.Store, raw integrationingest.RawWriter) (ChatProviderOut, error) {
	telegramConfig, ok, err := telegramProviderConfig(layout.Root)
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

func telegramProviderConfig(root string) (telegram.Config, bool, error) {
	credentials, ok, err := telegramconnector.Credentials()
	if err != nil {
		return telegram.Config{}, false, err
	}
	if !ok {
		credentials, ok, err = telegramProviderRuntimeEnvConfig(root)
		if err != nil || !ok {
			return telegram.Config{}, false, err
		}
	}
	return telegram.Config{APIID: credentials.APIID, APIHash: credentials.APIHash}, true, nil
}

func telegramProviderRuntimeEnvConfig(root string) (telegramconnector.ClientCredentials, bool, error) {
	envPath := runtimeenv.Path(root)
	id, _ := runtimeenv.Lookup(envPath, telegramconnector.EnvAppID)
	hash, _ := runtimeenv.Lookup(envPath, telegramconnector.EnvAppHash)
	return telegramconnector.ParseCredentials(id, hash)
}
