package materialize

import (
	"strings"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func recordAccountSlug(recordAccount string) string {
	account := integrations.NormalizeAlias(recordAccount)
	if account == "" {
		account = "unknown"
	}
	return account
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sourceMediaType(value string) string {
	if value != "" {
		return value
	}
	return "text/markdown"
}

func sourceReplay(account string, scopes ...integrations.ReplayScope) integrations.Replay {
	return integrations.Replay{Account: account, Scopes: scopes}
}

func sourceKey(entity, day string) integrations.SourceKey {
	return integrations.SourceKey{Entity: entity, Day: day}
}
