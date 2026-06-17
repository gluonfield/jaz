package provider

import (
	"sync"
	"testing"
)

type fakeLoader struct {
	configs map[string]ModelProviderConfig
}

func (f fakeLoader) CustomProviderConfigs() (map[string]ModelProviderConfig, error) {
	return f.configs, nil
}

func TestSourceMergesBaseAndCustoms(t *testing.T) {
	base := map[string]ModelProviderConfig{
		"openai": {Type: "openai", BaseURL: "https://api.openai.com/v1"},
	}
	src, err := NewSource(base, fakeLoader{configs: map[string]ModelProviderConfig{
		"groq": {Type: "openai-compatible", BaseURL: "https://groq/v1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := src.Providers()
	if _, ok := got["openai"]; !ok {
		t.Fatal("base provider missing")
	}
	if _, ok := got["groq"]; !ok {
		t.Fatal("custom provider missing")
	}
}

func TestSourceBaseWinsOverCustom(t *testing.T) {
	base := map[string]ModelProviderConfig{
		"openai": {Type: "openai", BaseURL: "https://api.openai.com/v1"},
	}
	src, err := NewSource(base, fakeLoader{configs: map[string]ModelProviderConfig{
		"openai": {BaseURL: "https://evil/v1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if url := src.Providers()["openai"].BaseURL; url != "https://api.openai.com/v1" {
		t.Fatalf("base provider must win, got %q", url)
	}
}

func TestProvidersReturnsIsolatedCopy(t *testing.T) {
	src, err := NewSource(map[string]ModelProviderConfig{"a": {BaseURL: "x"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := src.Providers()
	got["a"] = ModelProviderConfig{BaseURL: "mutated"}
	got["b"] = ModelProviderConfig{}
	if src.Providers()["a"].BaseURL != "x" {
		t.Fatal("mutating the returned map leaked into internal state")
	}
	if _, ok := src.Providers()["b"]; ok {
		t.Fatal("adding to the returned map leaked into internal state")
	}
}

// Run under -race: concurrent readers and reloaders must not race the merged map.
func TestSourceConcurrentReadReload(t *testing.T) {
	src, err := NewSource(
		map[string]ModelProviderConfig{"a": {}},
		fakeLoader{configs: map[string]ModelProviderConfig{"b": {}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = src.Providers()
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = src.Reload()
			}
		}()
	}
	wg.Wait()
}
