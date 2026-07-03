package connections

import (
	"context"
	"strings"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	connectionScopeContacts = "contacts"
	connectionScopeMessages = "messages"
	connectionScopeSend     = "send"
)

type ConnectionToolStore interface {
	ListConnections(context.Context, string) ([]integrations.Connection, error)
}

type mcpAccountSelection struct {
	Connections     []integrations.Connection
	Connection      integrations.Connection
	Connected       bool
	AccountRequired bool
	Text            string
}

func selectMCPConnection(ctx context.Context, store ConnectionToolStore, provider, providerName, account string) (mcpAccountSelection, error) {
	connections, err := store.ListConnections(ctx, provider)
	if err != nil {
		return mcpAccountSelection{}, err
	}
	connection, ok := selectConnection(connections, account)
	if ok {
		return mcpAccountSelection{Connections: connections, Connection: connection, Connected: true}, nil
	}
	if len(connections) > 1 {
		return mcpAccountSelection{
			Connections:     connections,
			Connected:       true,
			AccountRequired: true,
			Text:            mcpAccountRequiredText(providerName, connections),
		}, nil
	}
	return mcpAccountSelection{
		Connections: connections,
		Text:        providerName + " is not connected. Connect it in Settings > Connections.",
	}, nil
}

func mcpAccountRequiredText(providerName string, connections []integrations.Connection) string {
	var refs []string
	for _, connection := range connections {
		if ref := connection.AccountRef(); ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return "Multiple " + providerName + " accounts are connected. Specify the account alias, account id, or connection id."
	}
	return "Multiple " + providerName + " accounts are connected. Specify account as one of: " + strings.Join(refs, ", ") + "."
}

func connectionHasScope(connection integrations.Connection, scope string) bool {
	for _, value := range connection.Scopes {
		if strings.EqualFold(strings.TrimSpace(value), scope) {
			return true
		}
	}
	return false
}

func mcpScopeDeniedText(providerName, scope string) string {
	switch scope {
	case connectionScopeContacts:
		return providerName + " contact/search access is disabled for this account."
	case connectionScopeMessages:
		return providerName + " message read access is disabled for this account."
	case connectionScopeSend:
		return providerName + " message send access is disabled for this account."
	default:
		return providerName + " access is disabled for this account."
	}
}
