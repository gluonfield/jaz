package connections

import (
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestCatalogIncludesGmail(t *testing.T) {
	catalog := NewCatalog()
	plugins := catalog.ListPlugins()
	if len(plugins) != 6 {
		t.Fatalf("plugins = %#v", plugins)
	}
	for _, id := range []string{"deployink", "gmail", "google_calendar", "slack", "telegram", "whatsapp"} {
		plugin, ok := catalog.Plugin(id)
		if !ok || plugin.ID != id {
			t.Fatalf("%s plugin = %#v ok=%v", id, plugin, ok)
		}
	}

	deployink, ok := catalog.Plugin("deployink")
	if !ok {
		t.Fatal("deployink plugin missing")
	}
	if deployink.Name != "Deployink" {
		t.Fatalf("deployink name = %q", deployink.Name)
	}
	if deployink.Icon.Kind != integrations.PluginIconKindAsset || deployink.Icon.Value != "ink" {
		t.Fatalf("deployink icon = %#v", deployink.Icon)
	}
}

func TestNilCatalogIsEmpty(t *testing.T) {
	var catalog *Catalog
	if got := catalog.ListPlugins(); got != nil {
		t.Fatalf("plugins = %#v", got)
	}
	if _, ok := catalog.Plugin("gmail"); ok {
		t.Fatal("nil catalog returned plugin")
	}
}
