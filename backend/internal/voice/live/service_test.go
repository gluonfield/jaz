package live

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

type settingsStore struct {
	storage.SettingsStorage
	value json.RawMessage
}

func (s *settingsStore) LoadSetting(string, string) (storage.Setting, error) {
	if s.value == nil {
		return storage.Setting{}, storage.ErrSettingNotFound
	}
	return storage.Setting{Value: s.value}, nil
}

func (s *settingsStore) SaveSetting(_, _ string, value json.RawMessage) (storage.Setting, error) {
	s.value = value
	return storage.Setting{Value: value}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestConnectUsesEachProviderContract(t *testing.T) {
	for _, connection := range []string{ProviderOAuth, ProviderAPI} {
		t.Run(connection, func(t *testing.T) {
			store := &settingsStore{}
			service := NewService(store, func(id string) (Credential, error) {
				return Credential{Token: "secret-" + id, AccountID: "account"}, nil
			})
			if _, err := service.SaveSettings(Settings{Agent: "openai", Provider: connection, Voice: map[string]string{ProviderOAuth: "ember", ProviderAPI: "quartz"}[connection]}); err != nil {
				t.Fatal(err)
			}
			service.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "Bearer secret-"+connection {
					t.Fatal("wrong credential selected")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				var session struct {
					Audio struct {
						Output struct {
							Voice string `json:"voice"`
						} `json:"output"`
					} `json:"audio"`
					Model        string `json:"model"`
					Instructions string `json:"instructions"`
					Delegation   struct {
						Type string `json:"type"`
					} `json:"delegation"`
				}
				if err := json.Unmarshal(body["session"], &session); err != nil {
					t.Fatal(err)
				}
				if session.Audio.Output.Voice != map[string]string{ProviderOAuth: "ember", ProviderAPI: "quartz"}[connection] {
					t.Fatalf("selected voice did not reach session startup: %q", session.Audio.Output.Voice)
				}
				if session.Delegation.Type != "client" {
					t.Fatal("voice must delegate to Jaz, not launch a managed Responses agent")
				}
				if !strings.Contains(session.Instructions, "Current thread: check the directory") || !strings.Contains(session.Instructions, "Voice model: "+session.Model) {
					t.Fatal("startup context and voice identity must be sent before the media session starts")
				}
				answer := "v=0\r\nanswer"
				if connection == ProviderOAuth {
					if r.URL.Host != "chatgpt.com" || r.URL.Path != "/backend-api/codex/realtime/calls" || r.URL.Query().Get("architecture") != "avas" || session.Model != "gpt-live-1-codex" || body["sdp"] == nil || body["transport"] != nil {
						t.Fatal("subscription session contract changed")
					}
					if r.Header.Get("ChatGPT-Account-Id") != "account" || r.Header.Get("OpenAI-Alpha") != "quicksilver=v2" {
						t.Fatal("subscription headers missing")
					}
				} else {
					if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/live/sessions" || session.Model != "gpt-live-1" || body["transport"] == nil || body["sdp"] != nil {
						t.Fatal("public Live session contract changed")
					}
					if r.Header.Get("ChatGPT-Account-Id") != "" {
						t.Fatal("subscription account leaked into API request")
					}
					answer = `{"session":{"id":"sess_test"},"transport":{"type":"webrtc","sdp":"v=0\r\nanswer"}}`
				}
				return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(answer))}, nil
			})
			result, err := service.Connect(context.Background(), "v=0\r\noffer", "Current thread: check the directory")
			if err != nil || result.SDP != "v=0\r\nanswer" || result.Provider != connection {
				t.Fatalf("connection = %#v, %v", result, err)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "secret") || strings.Contains(string(store.value), "secret") {
				t.Fatal("credential escaped into client state")
			}
		})
	}
}

func TestExplicitProviderNeverFallsBack(t *testing.T) {
	store := &settingsStore{}
	service := NewService(store, func(id string) (Credential, error) {
		if id == ProviderOAuth {
			return Credential{}, errors.New("sign in")
		}
		return Credential{Token: "api"}, nil
	})
	status, err := service.Settings()
	if err != nil || status.Provider != ProviderAPI {
		t.Fatalf("initial selection: %#v %v", status, err)
	}
	if _, err := service.SaveSettings(Settings{Agent: "openai", Provider: ProviderOAuth}); err != nil {
		t.Fatal(err)
	}
	_, err = service.Connect(context.Background(), "v=0\r\noffer", "Current thread: check the directory")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("explicit subscription unexpectedly fell back: %v", err)
	}
}

func TestProviderErrorsDoNotExposeResponseSecrets(t *testing.T) {
	service := NewService(&settingsStore{}, func(string) (Credential, error) {
		return Credential{Token: "secret"}, nil
	})
	for _, code := range []int{401, 403, 429, 500} {
		service.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("secret provider payload"))}, nil
		})
		_, err := service.Connect(context.Background(), "v=0\r\noffer", "Current thread: check the directory")
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe provider error: %v", err)
		}
	}
}

func TestUnauthorizedRefreshesOnceAndPreservesTheSDPOffer(t *testing.T) {
	refreshes := 0
	service := NewService(&settingsStore{}, func(string) (Credential, error) {
		return Credential{Token: "expired", AccountID: "account", Refresh: func(context.Context) (Credential, error) {
			refreshes++
			return Credential{Token: "fresh", AccountID: "account"}, nil
		}}, nil
	})
	attempts := 0
	var original []byte
	service.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		if attempts == 1 {
			original = body
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("expired"))}, nil
		}
		if string(original) != string(body) || r.Header.Get("Authorization") != "Bearer fresh" {
			t.Fatal("refresh changed the request or reused the expired credential")
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader("v=0\r\nanswer"))}, nil
	})
	if _, err := service.Connect(context.Background(), "v=0\r\noffer", "Current thread: check the directory"); err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 || attempts != 2 {
		t.Fatalf("refreshes=%d attempts=%d", refreshes, attempts)
	}
}

func TestInvalidPersistedProviderCannotRouteCredentials(t *testing.T) {
	for _, value := range []string{
		`{"agent":"openai","provider":"typo"}`,
		`{"agent":"unsupported","provider":"openai"}`,
	} {
		service := NewService(&settingsStore{value: json.RawMessage(value)}, func(id string) (Credential, error) {
			if id != ProviderOAuth && id != ProviderAPI {
				t.Fatalf("resolved credentials for unknown provider %q", id)
			}
			return Credential{Token: "secret"}, nil
		})
		service.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid settings must be rejected before sending credentials")
			return nil, nil
		})
		_, err := service.Connect(context.Background(), "v=0\r\noffer", "")
		if !errors.Is(err, ErrInput) {
			t.Fatalf("Connect() error = %v", err)
		}
	}
}

func TestVoiceSelectionPersistsAndRejectsAnotherProvidersCatalog(t *testing.T) {
	store := &settingsStore{value: json.RawMessage(`{"agent":"openai","provider":"openai"}`)}
	service := NewService(store, func(string) (Credential, error) {
		return Credential{}, nil
	})
	status, err := service.Settings()
	if err != nil || status.Voice != "cove" {
		t.Fatalf("existing selection: %#v %v", status, err)
	}
	for _, provider := range []string{ProviderOAuth, ProviderAPI} {
		for _, voice := range voices(provider) {
			want := Settings{Agent: "openai", Provider: provider, Voice: voice}
			if _, err := service.SaveSettings(want); err != nil {
				t.Fatal(err)
			}
			got, err := service.Settings()
			if err != nil || got.Settings != want {
				t.Fatalf("roundtrip: %#v %v", got, err)
			}
		}
	}
	before := string(store.value)
	if _, err := service.SaveSettings(Settings{Agent: "openai", Provider: ProviderAPI, Voice: "cove"}); !errors.Is(err, ErrInput) {
		t.Fatalf("subscription voice accepted on public API: %v", err)
	}
	if string(store.value) != before {
		t.Fatal("rejected voice overwrote the selected voice")
	}
	status, err = service.SaveSettings(Settings{Agent: "openai", Provider: ProviderAPI})
	if err != nil || status.Voice != "marin" {
		t.Fatalf("provider switch default: %#v %v", status, err)
	}
}
