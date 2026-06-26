package connections

import (
	"strings"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func selectConnection(connections []integrations.Connection, account string) (integrations.Connection, bool) {
	account = strings.TrimSpace(account)
	if account == "" {
		if len(connections) == 1 {
			return connections[0], true
		}
		return integrations.Connection{}, false
	}
	for _, connection := range connections {
		if connectionMatches(connection, account) {
			return connection, true
		}
	}
	return integrations.Connection{}, false
}

func connectionMatches(connection integrations.Connection, account string) bool {
	accountNorm := integrations.NormalizeAlias(account)
	for _, value := range []string{connection.ID, connection.Alias, connection.AccountID, connection.AccountName} {
		if strings.EqualFold(strings.TrimSpace(value), account) {
			return true
		}
		if accountNorm != "" && integrations.NormalizeAlias(value) == accountNorm {
			return true
		}
	}
	return false
}
