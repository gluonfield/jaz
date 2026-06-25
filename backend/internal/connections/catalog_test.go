package connections

import "testing"

func TestCatalogIncludesGmail(t *testing.T) {
	catalog := NewCatalog()
	plugins := catalog.ListPlugins()
	if len(plugins) != 1 || plugins[0].ID != "gmail" {
		t.Fatalf("plugins = %#v", plugins)
	}
	plugin, ok := catalog.Plugin("gmail")
	if !ok || plugin.Name != "Gmail" {
		t.Fatalf("gmail plugin = %#v ok=%v", plugin, ok)
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
