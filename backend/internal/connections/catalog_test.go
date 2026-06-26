package connections

import "testing"

func TestCatalogIncludesGmail(t *testing.T) {
	catalog := NewCatalog()
	plugins := catalog.ListPlugins()
	if len(plugins) != 3 {
		t.Fatalf("plugins = %#v", plugins)
	}
	for _, id := range []string{"gmail", "telegram", "whatsapp"} {
		plugin, ok := catalog.Plugin(id)
		if !ok || plugin.ID != id {
			t.Fatalf("%s plugin = %#v ok=%v", id, plugin, ok)
		}
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
