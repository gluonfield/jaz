package providerstore

import (
	"encoding/json"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

// memStore is an in-memory SettingsStorage for unit tests.
type memStore struct {
	data map[string]json.RawMessage
}

func newMemStore() *memStore { return &memStore{data: map[string]json.RawMessage{}} }

func memKey(ns, k string) string { return ns + "\x00" + k }

func (m *memStore) LoadSetting(ns, k string) (storage.Setting, error) {
	v, ok := m.data[memKey(ns, k)]
	if !ok {
		return storage.Setting{}, storage.ErrSettingNotFound
	}
	return storage.Setting{Namespace: ns, Key: k, Value: v}, nil
}

func (m *memStore) SaveSetting(ns, k string, value json.RawMessage) (storage.Setting, error) {
	m.data[memKey(ns, k)] = append(json.RawMessage(nil), value...)
	return storage.Setting{Namespace: ns, Key: k, Value: value}, nil
}

func (m *memStore) DeleteSetting(ns, k string) error {
	delete(m.data, memKey(ns, k))
	return nil
}

func (m *memStore) ListSettings(ns string) ([]storage.Setting, error) { return nil, nil }

func TestCreateGetUpdateDelete(t *testing.T) {
	store := newMemStore()

	rec, err := Create(store, Input{Label: "Groq", BaseURL: "https://api.groq.com/openai/v1/"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "groq" {
		t.Fatalf("id = %q, want groq", rec.ID)
	}
	if rec.APIKeyEnv != "JAZ_PROVIDER_GROQ_API_KEY" {
		t.Fatalf("api_key_env = %q", rec.APIKeyEnv)
	}
	if len(rec.Capabilities) != 1 || rec.Capabilities[0] != "chat_completions" {
		t.Fatalf("capabilities = %#v", rec.Capabilities)
	}
	if rec.BaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("base_url not trimmed: %q", rec.BaseURL)
	}

	got, ok, err := Get(store, "groq")
	if err != nil || !ok || got.Label != "Groq" {
		t.Fatalf("get = %#v ok=%v err=%v", got, ok, err)
	}

	upd, err := Update(store, "groq", Input{
		Label:        "Groq Cloud",
		BaseURL:      "https://api.groq.com/openai/v1",
		DefaultModel: "llama-3.1-70b",
		Capabilities: []string{"responses", "chat_completions"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Label != "Groq Cloud" || upd.DefaultModel != "llama-3.1-70b" {
		t.Fatalf("update did not apply: %#v", upd)
	}
	if cfg := upd.Config(); cfg.DefaultModel != "llama-3.1-70b" {
		t.Fatalf("config default_model = %q", cfg.DefaultModel)
	}
	if cfg := upd.Config(); len(cfg.Capabilities) != 2 || cfg.Capabilities[0] != "chat_completions" || cfg.Capabilities[1] != "responses" {
		t.Fatalf("config capabilities = %#v", cfg.Capabilities)
	}
	if upd.ID != "groq" || upd.APIKeyEnv != rec.APIKeyEnv {
		t.Fatal("id changed or remote api_key_env was not stable across updates")
	}

	removed, err := Delete(store, "groq")
	if err != nil || removed.ID != "groq" {
		t.Fatalf("delete = %#v err=%v", removed, err)
	}
	list, _ := List(store)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}

func TestCreateLoopbackProviderDoesNotRequireKey(t *testing.T) {
	rec, err := Create(newMemStore(), Input{Label: "Ollama", BaseURL: "http://127.0.0.1:11434/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.APIKeyEnv != "JAZ_PROVIDER_OLLAMA_2_API_KEY" {
		t.Fatalf("api_key_env = %q", rec.APIKeyEnv)
	}
	cfg := rec.Config()
	if cfg.APIKeyEnv != "" {
		t.Fatalf("config api_key_env = %q, want empty", cfg.APIKeyEnv)
	}
}

func TestLoopbackProviderConfigIgnoresLegacyKeyEnv(t *testing.T) {
	rec := CustomProvider{
		ID:        "ollama-2",
		Label:     "Ollama",
		BaseURL:   "http://127.0.0.1:11434/v1",
		APIKeyEnv: "JAZ_PROVIDER_OLLAMA_2_API_KEY",
	}
	if cfg := rec.Config(); cfg.APIKeyEnv != "" {
		t.Fatalf("config api_key_env = %q, want empty", cfg.APIKeyEnv)
	}
}

func TestListKeepsStableProviderKeyEnv(t *testing.T) {
	store := newMemStore()
	_, err := store.SaveSetting(SettingsNamespace, CustomKey, json.RawMessage(`{"providers":[{"id":"ollama-2","label":"Ollama","base_url":"http://localhost:11434/v1","api_type":"openai-compatible","api_key_env":"JAZ_PROVIDER_OLLAMA_2_API_KEY","created_at":"2026-07-04T14:11:38Z","updated_at":"2026-07-04T14:11:38Z"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	records, err := List(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].APIKeyEnv != "JAZ_PROVIDER_OLLAMA_2_API_KEY" {
		t.Fatalf("api_key_env = %q", records[0].APIKeyEnv)
	}
	if len(records[0].Capabilities) != 1 || records[0].Capabilities[0] != "chat_completions" {
		t.Fatalf("legacy capabilities = %#v", records[0].Capabilities)
	}
	if cfg := records[0].Config(); cfg.APIKeyEnv != "" {
		t.Fatalf("config api_key_env = %q, want empty", cfg.APIKeyEnv)
	}
}

func TestListRejectsInvalidPersistedCapability(t *testing.T) {
	store := newMemStore()
	_, err := store.SaveSetting(SettingsNamespace, CustomKey, json.RawMessage(`{"providers":[{"id":"bad","label":"Bad","base_url":"https://bad.test/v1","capabilities":["response"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := List(store); err == nil {
		t.Fatal("invalid persisted capability was accepted")
	}
}

func TestUpdateKeepsStableKeyEnvAcrossBaseURLChanges(t *testing.T) {
	store := newMemStore()
	rec, err := Create(store, Input{Label: "Local", BaseURL: "http://localhost:11434/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.APIKeyEnv != "JAZ_PROVIDER_LOCAL_API_KEY" {
		t.Fatalf("api_key_env = %q", rec.APIKeyEnv)
	}
	remote, err := Update(store, rec.ID, Input{Label: "Local", BaseURL: "https://llm.internal/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if remote.APIKeyEnv != "JAZ_PROVIDER_LOCAL_API_KEY" {
		t.Fatalf("remote api_key_env = %q", remote.APIKeyEnv)
	}
	local, err := Update(store, rec.ID, Input{Label: "Local", BaseURL: "http://127.0.0.1:11434/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if local.APIKeyEnv != "JAZ_PROVIDER_LOCAL_API_KEY" {
		t.Fatalf("loopback api_key_env = %q", local.APIKeyEnv)
	}
	if cfg := local.Config(); cfg.APIKeyEnv != "" {
		t.Fatalf("loopback config api_key_env = %q, want empty", cfg.APIKeyEnv)
	}
}

func TestCreateRejectsReservedAndDedupes(t *testing.T) {
	store := newMemStore()

	// "OpenAI" slugs to the reserved built-in id "openai" → must not collide.
	reserved, err := Create(store, Input{Label: "OpenAI", BaseURL: "https://x/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if reserved.ID == "openai" {
		t.Fatalf("custom must not take a reserved id, got %q", reserved.ID)
	}

	a, _ := Create(store, Input{Label: "Groq", BaseURL: "https://a/v1"})
	b, _ := Create(store, Input{Label: "Groq", BaseURL: "https://b/v1"})
	if a.ID == b.ID {
		t.Fatalf("duplicate labels must get distinct ids, both %q", a.ID)
	}
}

func TestValidateInputRejectsBadInput(t *testing.T) {
	cases := []Input{
		{Label: "", BaseURL: "https://x/v1"},
		{Label: "x", BaseURL: ""},
		{Label: "x", BaseURL: "ftp://x/v1"},
		{Label: "x", BaseURL: "https://x/v1", Capabilities: []string{"completions"}},
		{Label: "x", BaseURL: "not a url at all"},
	}
	for i, in := range cases {
		if _, err := Create(newMemStore(), in); err == nil {
			t.Fatalf("case %d: expected validation error for %#v", i, in)
		}
	}
}

func TestDeleteNotFound(t *testing.T) {
	if _, err := Delete(newMemStore(), "nope"); err == nil {
		t.Fatal("expected not found error")
	}
}
