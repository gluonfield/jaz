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
	if err := b.requireEnabled(); err != nil {
		return ActionOutput{}, err
	}
	return b.DesktopBackend.Call(ctx, input)
}

func (b *ConfiguredBackend) ActivePageOrigin(ctx context.Context, sessionID string) (string, error) {
	if err := b.requireEnabled(); err != nil {
		return "", err
	}
	return b.DesktopBackend.ActivePageOrigin(ctx, sessionID)
}

func (b *ConfiguredBackend) BlindFill(ctx context.Context, sessionID string, req VaultFillRequest) (VaultFillResult, error) {
	if err := b.requireEnabled(); err != nil {
		return VaultFillResult{}, err
	}
	return b.DesktopBackend.BlindFill(ctx, sessionID, req)
}

func (b *ConfiguredBackend) requireEnabled() error {
	config, err := settings.LoadBrowserSettings(b.store)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return errors.New("browser tools are disabled in Browser settings")
	}
	return nil
}
