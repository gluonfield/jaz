package connections

import (
	"context"
	"errors"

	calendarconnector "github.com/wins/jaz/backend/internal/connectors/calendar"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	defaultCalendarEventLimit = 10
	maxCalendarEventLimit     = 50
)

type CalendarToolStore interface {
	integrationoauth.Store
	ListConnections(context.Context, string) ([]integrations.Connection, error)
}

type calendarToolSession struct {
	api        calendarconnector.APIClient
	connection integrations.Connection
	accounts   []integrations.Connection
}

func (t *CalendarMCPTools) session(ctx context.Context, account string) (calendarToolSession, bool, error) {
	connections, err := t.calendarConnections(ctx)
	if err != nil {
		return calendarToolSession{}, false, err
	}
	session := calendarToolSession{accounts: connections}
	connection, ok := selectConnection(connections, account)
	if !ok {
		return session, false, nil
	}
	client, err := (integrationoauth.Refresher{Store: t.store}).Client(ctx, connection.ID)
	if errors.Is(err, integrationoauth.ErrTokenNotFound) {
		return session, false, nil
	}
	if err != nil {
		return calendarToolSession{}, false, err
	}
	session.api = calendarconnector.APIClient{HTTP: client, BaseURL: t.apiBaseURL}
	session.connection = connection
	return session, true, nil
}

func (t *CalendarMCPTools) calendarConnections(ctx context.Context) ([]integrations.Connection, error) {
	return t.store.ListConnections(ctx, calendarconnector.ProviderID)
}

func calendarEventLimit(limit int) int {
	if limit <= 0 {
		return defaultCalendarEventLimit
	}
	return min(limit, maxCalendarEventLimit)
}
