package telegram

import (
	"context"
	"errors"
	"os"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func (p *Provider) Disconnect(ctx context.Context, connection integrations.Connection) error {
	if err := p.stopClient(ctx, connection.ID); err != nil {
		return err
	}
	return errors.Join(
		removeFile(p.sessionPath(connection.ID)),
		removeFile(p.backfillMarkerPath(connection.ID)),
		removeFile(p.backfillStatePath(connection.ID)),
		removeFile(p.contactsMarkerPath(connection.ID)),
	)
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
