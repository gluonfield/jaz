package connections

import (
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/internal/sourcepaths"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func (s *Service) relevantPaths(connection integrations.Connection) []AgentPath {
	account := accountPathComponent(connection)
	if account == "" {
		return nil
	}
	switch connection.Provider {
	case gmailconnector.ProviderID:
		return []AgentPath{memoryPrefix(
			sourcepaths.EmailMessagesPrefix(gmailconnector.ProviderID, account),
			"Materialized Gmail message source pages; search this prefix, then read exact pages returned by memory search.",
		)}
	case telegramconnector.ProviderID:
		return chatPaths(telegramconnector.ProviderID, account)
	case whatsappconnector.ProviderID:
		return chatPaths(whatsappconnector.ProviderID, account)
	default:
		return nil
	}
}

func chatPaths(provider, account string) []AgentPath {
	return []AgentPath{
		memoryPage(sourcepaths.ChatContactPath(provider, account), "Contact index for resolving names, handles, and chat IDs."),
		memoryPrefix(sourcepaths.ChatConversationsPrefix(provider, account), "Materialized chat day source pages; search this prefix, then read exact pages returned by memory search."),
	}
}

func memoryPage(pagePath, explanation string) AgentPath {
	return AgentPath{
		Path:        pagePath,
		Kind:        AgentPathKindMemoryPage,
		Explanation: explanation,
	}
}

func memoryPrefix(prefix, explanation string) AgentPath {
	return AgentPath{
		Path:        prefix,
		Kind:        AgentPathKindMemoryPrefix,
		Explanation: explanation,
	}
}

func accountPathComponent(connection integrations.Connection) string {
	account := integrations.NormalizeAlias(connection.AccountID)
	if account != "" {
		return account
	}
	account = integrations.NormalizeAlias(connection.AccountRef())
	if account != "" {
		return account
	}
	return integrations.NormalizeAlias(connection.ID)
}
