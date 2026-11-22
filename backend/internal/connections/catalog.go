package connections

import (
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/connectors/calendar"
	"github.com/wins/jaz/backend/internal/connectors/drive"
	"github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type Catalog struct {
	plugins []integrations.Plugin
	clients map[string]mcpconfig.OAuthConfig
}

func NewCatalog(clients map[string]mcpconfig.OAuthConfig) *Catalog {
	return &Catalog{
		clients: clients,
		plugins: []integrations.Plugin{
			calendar.Plugin(),
			drive.Plugin(),
			gmail.Plugin(),
			slack.Plugin(),
			telegram.Plugin(),
			whatsapp.Plugin(),
		},
	}
}

func (c *Catalog) OAuthClientByID(id string) mcpconfig.OAuthConfig {
	if c == nil {
		return mcpconfig.OAuthConfig{}
	}
	return c.clients[id]
}

func (c *Catalog) IsNative(id string) bool {
	_, ok := c.Plugin(id)
	return ok
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
