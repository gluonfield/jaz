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
	catalog := &Catalog{clients: clients, plugins: []integrations.Plugin{
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
	}}
	for i, plugin := range catalog.plugins {
		if !mcpConnectionPlugin(plugin) || !plugin.RemoteMCP.OAuthSecrets {
			continue
		}
		client := catalog.MCPOAuthClient(plugin)
		if !client.UsesLoopback() && (client.ClientID == "" || strings.TrimSpace(os.Getenv(client.ClientSecretEnvVar)) == "") {
			catalog.plugins[i].Implementation.Status = "setup_required"
			catalog.plugins[i].Implementation.Reason = fmt.Sprintf("%s requires a registered OAuth app. Configure a public client ID for local sign-in or ask your Jaz administrator to configure its credentials.", plugin.Name)
		}
	}
	return catalog
}

func (c *Catalog) MCPOAuthClient(plugin integrations.Plugin) mcpconfig.OAuthConfig {
	return c.clients[cmp.Or(plugin.Provider.ID, plugin.ID)]
}

func (c *Catalog) forSignIn(plugin integrations.Plugin, loopbackAllowed bool) integrations.Plugin {
	if !loopbackAllowed && plugin.Available() && mcpConnectionPlugin(plugin) && c.MCPOAuthClient(plugin).UsesLoopback() {
		plugin.Implementation.Status = "local_sign_in_required"
		plugin.Implementation.Reason = fmt.Sprintf("Sign in to %s from the device running the local Jaz backend.", plugin.Name)
	}
	return plugin
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
