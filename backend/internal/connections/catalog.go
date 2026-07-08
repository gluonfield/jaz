package connections

import (
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/connectors/calendar"
	"github.com/wins/jaz/backend/internal/connectors/deployink"
	"github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/connectors/jaz"
	"github.com/wins/jaz/backend/internal/connectors/slack"
	"github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type Catalog struct {
	plugins []integrations.Plugin
}

func NewCatalog() *Catalog {
	return &Catalog{plugins: []integrations.Plugin{
		calendar.Plugin(),
		deployink.Plugin(),
		gmail.Plugin(),
		jaz.Plugin(),
		slack.Plugin(),
		telegram.Plugin(),
		whatsapp.Plugin(),
	}}
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

// HasInternalPlugin reports whether the catalog declares Jaz's built-in tool
// surface; MCP wiring exposes the jaztools server only when it does.
func (c *Catalog) HasInternalPlugin() bool {
	if c == nil {
		return false
	}
	for _, plugin := range c.plugins {
		if plugin.Internal() {
			return true
		}
	}
	return false
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
