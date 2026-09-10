package browsercontrol

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type ConfiguredBackend struct {
	*ExtensionBridge
	Desktop *DesktopBackend
	store   storage.SettingsStorage
}

func NewConfiguredBackend(profileRoot string, store storage.SettingsStorage) *ConfiguredBackend {
	extension := NewExtensionBridge(NewLocalBackend(profileRoot), func() bool {
		config, err := settings.LoadBrowserSettings(store)
		return err != nil || settings.BrowserUsesExtension(config)
	})
	return &ConfiguredBackend{ExtensionBridge: extension, Desktop: NewDesktopBackend(), store: store}
}

func (b *ConfiguredBackend) Call(ctx context.Context, input ActionInput) (ActionOutput, error) {
	config, err := settings.LoadBrowserSettings(b.store)
	if err != nil {
		return ActionOutput{}, err
	}
	if !config.Enabled {
		return ActionOutput{}, errors.New("browser tools are disabled in Browser settings")
	}
	if config.Mode == settings.BrowserModeDesktop {
		return b.Desktop.Call(ctx, input)
	}
	return b.ExtensionBridge.Call(ctx, input)
}

func (b *ConfiguredBackend) Close() error {
	return errors.Join(b.Desktop.Close(), b.ExtensionBridge.Close())
}
