package connections

import "testing"

func TestCatalogIncludesOnlyNativeIntegrations(t *testing.T) {
	catalog := NewCatalog(nil)
	want := []string{"gmail", "google_calendar", "google_drive", "slack", "telegram", "whatsapp"}
	if got := catalog.ListPlugins(); len(got) != len(want) {
		t.Fatalf("catalog count = %d, want %d", len(got), len(want))
	}
	for _, id := range want {
		plugin, ok := catalog.Plugin(id)
		if !ok || plugin.ID != id || !catalog.IsNative(id) {
			t.Fatalf("%s plugin = %#v ok=%v", id, plugin, ok)
		}
	}
	for _, removed := range []string{"deployink", "granola", "zoom", "tasks", "crm"} {
		if _, ok := catalog.Plugin(removed); ok || catalog.IsNative(removed) {
			t.Fatalf("%s should no longer be hardcoded in native catalog", removed)
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
