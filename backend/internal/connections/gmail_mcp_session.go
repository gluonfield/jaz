package connections

import (
	"context"
	"errors"
	"strings"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	defaultGmailSearchLimit = 10
	maxGmailSearchLimit     = 20
	defaultGmailThreadLimit = 10
	maxGmailThreadLimit     = 20
	maxGmailBodyChars       = 6000
)

type GmailToolStore interface {
	integrationoauth.Store
	ListConnections(context.Context, string) ([]integrations.Connection, error)
}

type gmailToolSession struct {
	api        gmailconnector.APIClient
	connection integrations.Connection
	accounts   []integrations.Connection
}

func (t *GmailMCPTools) session(ctx context.Context, account string) (gmailToolSession, bool, error) {
	connections, err := t.gmailConnections(ctx)
	if err != nil {
		return gmailToolSession{}, false, err
	}
	session := gmailToolSession{accounts: connections}
	connection, ok := selectGmailConnection(connections, account)
	if !ok {
		return session, false, nil
	}
	client, err := (integrationoauth.Refresher{Store: t.store}).Client(ctx, connection.ID)
	if errors.Is(err, integrationoauth.ErrTokenNotFound) {
		return session, false, nil
	}
	if err != nil {
		return gmailToolSession{}, false, err
	}
	session.api = gmailconnector.APIClient{HTTP: client, BaseURL: t.apiBaseURL}
	session.connection = connection
	return session, true, nil
}

func (t *GmailMCPTools) gmailConnections(ctx context.Context) ([]integrations.Connection, error) {
	connections, err := t.store.ListConnections(ctx, gmailconnector.ProviderID)
	if err != nil {
		return nil, err
	}
	return connections, nil
}

func selectGmailConnection(connections []integrations.Connection, account string) (integrations.Connection, bool) {
	return selectConnection(connections, account)
}

func gmailAccountRequiredText(connections []integrations.Connection) string {
	var refs []string
	for _, connection := range connections {
		if ref := connection.AccountRef(); ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return "Multiple Gmail accounts are connected. Specify the account alias, email address, or connection id."
	}
	return "Multiple Gmail accounts are connected. Specify account as one of: " + strings.Join(refs, ", ") + "."
}

func gmailSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultGmailSearchLimit
	}
	return min(limit, maxGmailSearchLimit)
}

func gmailThreadLimit(limit int) int {
	if limit <= 0 {
		return defaultGmailThreadLimit
	}
	return min(limit, maxGmailThreadLimit)
}

func gmailThreadIDType(value string) gmailconnector.IDType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(gmailconnector.IDTypeMessage):
		return gmailconnector.IDTypeMessage
	case string(gmailconnector.IDTypeThread):
		return gmailconnector.IDTypeThread
	default:
		return ""
	}
}
