package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	cursordb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationcursors"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func (s *Store) LoadIntegrationCursor(ctx context.Context, connectionID, kind string) (integrations.Cursor, bool, error) {
	connectionID = strings.TrimSpace(connectionID)
	kind = strings.TrimSpace(kind)
	if connectionID == "" || kind == "" {
		return integrations.Cursor{}, false, nil
	}
	row, err := cursordb.New(s.db).LoadCursor(ctx, cursordb.LoadCursorParams{
		ConnectionID: connectionID,
		Kind:         kind,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return integrations.Cursor{}, false, nil
	}
	if err != nil {
		return integrations.Cursor{}, false, err
	}
	return integrations.Cursor{
		Kind:  row.Kind,
		Value: json.RawMessage(row.ValueJson),
	}, true, nil
}

func (s *Store) SaveIntegrationCursor(ctx context.Context, connectionID string, cursor integrations.Cursor) error {
	connectionID = strings.TrimSpace(connectionID)
	cursor.Kind = strings.TrimSpace(cursor.Kind)
	if connectionID == "" || cursor.Kind == "" {
		return nil
	}
	if len(cursor.Value) == 0 {
		cursor.Value = json.RawMessage(`{}`)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cursordb.New(s.db).SaveCursor(ctx, cursordb.SaveCursorParams{
		ConnectionID: connectionID,
		Kind:         cursor.Kind,
		ValueJson:    string(cursor.Value),
		UpdatedAtMs:  timeToMs(time.Now().UTC()),
	})
}
