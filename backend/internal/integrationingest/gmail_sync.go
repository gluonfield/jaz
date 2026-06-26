package integrationingest

import (
	"context"
	"errors"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	defaultGmailSyncInterval     = 5 * time.Minute
	defaultGmailSyncPagesPerTick = 4
	defaultGmailHistoricalWindow = 365 * 24 * time.Hour
)

type GmailSyncStore interface {
	integrationoauth.Store
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	LoadIntegrationCursor(context.Context, string, string) (integrations.Cursor, bool, error)
	SaveIntegrationCursor(context.Context, string, integrations.Cursor) error
}

type GmailSyncer struct {
	Store            GmailSyncStore
	Writer           RawWriter
	Interval         time.Duration
	MaxPagesPerTick  int
	APIBaseURL       string
	HistoricalWindow time.Duration
	Now              func() time.Time
}

func (s GmailSyncer) PollInterval() time.Duration {
	return s.interval()
}

func (s GmailSyncer) SyncOnce(ctx context.Context) error {
	connections, err := s.Store.ListConnections(ctx, gmailconnector.ProviderID)
	if err != nil {
		return err
	}
	var firstErr error
	for _, connection := range connections {
		if err := s.syncConnection(ctx, connection); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s GmailSyncer) syncConnection(ctx context.Context, connection integrations.Connection) error {
	client, err := (integrationoauth.Refresher{Store: s.Store}).Client(ctx, connection.ID)
	if errors.Is(err, integrationoauth.ErrTokenNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	api := gmailconnector.APIClient{HTTP: client, BaseURL: s.APIBaseURL}
	cursor, _, err := s.Store.LoadIntegrationCursor(ctx, connection.ID, gmailconnector.CursorKindSync)
	if err != nil {
		return err
	}
	for range s.pagesPerTick() {
		state, err := gmailconnector.DecodeSyncCursor(cursor)
		if err != nil {
			return err
		}
		mode := gmailObserveMode(state)
		result, err := api.Observe(ctx, integrations.ObserveRequest{
			Connection: connection,
			Cursor:     cursor,
			Mode:       mode,
			Since:      s.observeSince(mode),
		})
		if err != nil {
			return err
		}
		if len(result.Records) > 0 {
			records, err := cleanGmailSyncRecords(result.Records)
			if err != nil {
				return err
			}
			if _, err := s.Writer.WriteGmailMessages(ctx, records); err != nil {
				return err
			}
		}
		if result.Cursor.Empty() {
			return nil
		}
		if err := s.Store.SaveIntegrationCursor(ctx, connection.ID, result.Cursor); err != nil {
			return err
		}
		cursor = result.Cursor
		next, err := gmailconnector.DecodeSyncCursor(cursor)
		if err != nil {
			return err
		}
		if !gmailHasMorePages(next) {
			return nil
		}
	}
	return nil
}

func gmailObserveMode(cursor gmailconnector.SyncCursor) integrations.ObserveMode {
	if cursor.BackfillComplete {
		return integrations.ObserveModeIncremental
	}
	return integrations.ObserveModeBackfill
}

func gmailHasMorePages(cursor gmailconnector.SyncCursor) bool {
	if !cursor.BackfillComplete {
		return cursor.BackfillPageToken != ""
	}
	return cursor.HistoryPageToken != ""
}

func (s GmailSyncer) interval() time.Duration {
	if s.Interval > 0 {
		return s.Interval
	}
	return defaultGmailSyncInterval
}

func (s GmailSyncer) pagesPerTick() int {
	if s.MaxPagesPerTick > 0 {
		return s.MaxPagesPerTick
	}
	return defaultGmailSyncPagesPerTick
}

func (s GmailSyncer) observeSince(mode integrations.ObserveMode) time.Time {
	if mode != integrations.ObserveModeBackfill {
		return time.Time{}
	}
	window := s.historicalWindow()
	if window < 0 {
		return time.Time{}
	}
	now := s.now()
	return now.Add(-window)
}

func (s GmailSyncer) historicalWindow() time.Duration {
	if s.HistoricalWindow != 0 {
		return s.HistoricalWindow
	}
	return defaultGmailHistoricalWindow
}

func (s GmailSyncer) now() time.Time {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	return now
}
