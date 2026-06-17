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
	if rec.APIType != APITypeOpenAICompatible {
		t.Fatalf("api_type = %q", rec.APIType)
	}
	if rec.BaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("base_url not trimmed: %q", rec.BaseURL)
	}

	got, ok, err := Get(store, "groq")
	if err != nil || !ok || got.Label != "Groq" {
		t.Fatalf("get = %#v ok=%v err=%v", got, ok, err)
	}

	upd, err := Update(store, "groq", Input{Label: "Groq Cloud", BaseURL: "https://api.groq.com/openai/v1", DefaultModel: "llama-3.1-70b"})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Label != "Groq Cloud" || upd.DefaultModel != "llama-3.1-70b" {
		t.Fatalf("update did not apply: %#v", upd)
	}
	if upd.ID != "groq" || upd.APIKeyEnv != rec.APIKeyEnv {
		t.Fatal("id and api_key_env must be immutable across updates")
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
		{Label: "x", BaseURL: "https://x/v1", APIType: "anthropic"},
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
