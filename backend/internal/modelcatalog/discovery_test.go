package modelcatalog

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/providerstore"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestDiscoveryUsesStoredRoutingAndKeepsOpaqueIDsWhenSaving(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer private-test-key" || r.Header.Get("X-Session") != "routing" {
			t.Errorf("catalog request did not carry stored routing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"markoff/gpt-oss"},{"id":"vendor/another/model"}]}`))
	}))
	defer server.Close()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := providerstore.Create(store, providerstore.Input{Label: "Markoff", BaseURL: server.URL, Headers: map[string]string{"X-Session": "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := false
	discovery := Discovery{Store: store, LookupKey: func(string) string { return "private-test-key" }, Reload: func() error { reloaded = true; return nil }}
	result, err := discovery.Fetch(t.Context(), DiscoveryInput{ProviderID: record.ID, Save: true})
	if err != nil || !result.Saved || !reloaded {
		t.Fatalf("discovery result: %+v, %v", result, err)
	}
	saved, _, err := providerstore.Get(store, record.ID)
	if err != nil || saved.ModelsURL != "" || len(saved.Models) != 2 || saved.Models[0] != "markoff/gpt-oss" || saved.Models[1] != "vendor/another/model" {
		t.Fatalf("saved native IDs changed: %v, %v", saved.Models, err)
	}
	keyRead := false
	discovery.LookupKey = func(string) string { keyRead = true; return "private-test-key" }
	_, err = discovery.Fetch(t.Context(), DiscoveryInput{ProviderID: record.ID, ModelsURL: "https://unconfigured.example/models"})
	if !errors.Is(err, ErrDiscoveryInput) || keyRead {
		t.Fatalf("stored key could be forwarded to changed endpoint: %v", err)
	}
}
