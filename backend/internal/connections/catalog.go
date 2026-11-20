package connections

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/connectors/calendar"
	"github.com/wins/jaz/backend/internal/connectors/deployink"
	"github.com/wins/jaz/backend/internal/connectors/drive"
	"github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/connectors/granola"
	"github.com/wins/jaz/backend/internal/connectors/jazapps"
	"github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/internal/connectors/zoom"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type Catalog struct {
	plugins []integrations.Plugin
	clients map[string]mcpconfig.OAuthConfig
}

func NewCatalog(clients map[string]mcpconfig.OAuthConfig) *Catalog {
	plugins := []integrations.Plugin{
		calendar.Plugin(),
		deployink.Plugin(),
		drive.Plugin(),
		gmail.Plugin(),
		granola.Plugin(),
		jazapps.Tasks(),
		jazapps.CRM(),
		slack.Plugin(),
		telegram.Plugin(),
		whatsapp.Plugin(),
		zoom.Plugin(),
	}
	for i, plugin := range plugins {
		if !mcpConnectionPlugin(plugin) || !plugin.RemoteMCP.OAuthSecrets {
			continue
		}
		client := clients[cmp.Or(plugin.Provider.ID, plugin.ID)]
		if client.ClientID == "" || strings.TrimSpace(os.Getenv(client.ClientSecretEnvVar)) == "" {
			plugins[i].Implementation.Status = "setup_required"
			plugins[i].Implementation.Reason = fmt.Sprintf("%s requires a registered OAuth app. Ask your Jaz administrator to configure its client ID and secret.", plugin.Name)
		}
	}
	return &Catalog{plugins: plugins, clients: clients}
}

func (c *Catalog) MCPOAuthClient(provider string) mcpconfig.OAuthConfig {
	return c.clients[provider]
}

func (c *Catalog) ListPlugins() []integrations.Plugin {
	if c == nil {
		return nil
	}
	out := slices.Clone(c.plugins)
	slices.SortFunc(out, func(a, b integrations.Plugin) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func (c *Catalog) Plugin(id string) (integrations.Plugin, bool) {
	if c == nil {
		return integrations.Plugin{}, false
	}
	for _, plugin := range c.plugins {
		if plugin.ID == id {
			return plugin, true
		}
	}
	return integrations.Plugin{}, false
}
