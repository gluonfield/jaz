package browsercontrol

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type ConfiguredBackend struct {
	*DesktopBackend
	store storage.SettingsStorage
}

func NewConfiguredBackend(store storage.SettingsStorage) *ConfiguredBackend {
	return &ConfiguredBackend{DesktopBackend: NewDesktopBackend(), store: store}
}

func (b *ConfiguredBackend) Call(ctx context.Context, input ActionInput) (ActionOutput, error) {
	config, err := settings.LoadBrowserSettings(b.store)
	if err != nil {
		return ActionOutput{}, err
	}
	if !config.Enabled {
		return ActionOutput{}, errors.New("browser tools are disabled in Browser settings")
	}
	return b.DesktopBackend.Call(ctx, input)
}
