package voice

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/voice/live"
)

type voiceTransport func(*http.Request) (*http.Response, error)

func (f voiceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestConnectUsesCurrentBotIdentity(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := live.NewService(store, func(string) (live.Credential, error) {
		return live.Credential{Token: "fixture"}, nil
	})
	handler := NewHandler(service, nil)
	var instructions string
	original := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
	http.DefaultTransport = voiceTransport(func(r *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Session struct {
				Instructions string `json:"instructions"`
			} `json:"session"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatal(err)
		}
		instructions = body.Session.Instructions
		t.Logf("Provider %s session request: %s", r.URL.Host, data)
		answer := "v=0\r\nanswer"
		if r.URL.Host == "api.openai.com" {
			answer = `{"transport":{"sdp":"v=0\r\nanswer"}}`
		}
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(answer))}, nil
	})
	for _, provider := range []string{live.ProviderOAuth, live.ProviderAPI} {
		t.Run(provider, func(t *testing.T) {
			if _, err := service.SaveSettings(live.Settings{Agent: "openai", Provider: provider}); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"Prime", "CAS soft", "ordinary"} {
				t.Run(name, func(t *testing.T) {
					input := storage.CreateSession{Title: name, Slug: name, SourceType: storage.SourceBot}
					if name == "ordinary" {
						input.SourceType = ""
						input.Title = "Prime"
					}
					session, err := store.CreateSession(input)
					if err != nil {
						t.Fatal(err)
					}
					if input.SourceType == storage.SourceBot {
						if err := store.SaveBot(storage.BotRecord{ThreadID: session.ID, Kind: bots.KindBot, Prompt: "Private agent instructions."}); err != nil {
							t.Fatal(err)
						}
						prompt, err := bots.Prompt(store, session)
						if err != nil {
							t.Fatal(err)
						}
						t.Logf("Selected agent identity: %s", strings.SplitN(prompt.Text(), "\n", 2)[0])
					}
					connect := func(want string) {
						t.Helper()
						chatContext := "Current chat context (background only; earlier actions may already be complete):\nThread: " + name
						body, err := json.Marshal(map[string]string{"session_id": session.ID, "sdp": "v=0\r\noffer", "context": chatContext})
						if err != nil {
							t.Fatal(err)
						}
						response := httptest.NewRecorder()
						handler.Connect(response, httptest.NewRequest(http.MethodPost, "/v1/voice/connect", bytes.NewReader(body)))
						if response.Code != http.StatusCreated {
							t.Fatalf("connect = %d: %s", response.Code, response.Body.String())
						}
						if !strings.HasPrefix(instructions, "You are "+want+",") || (want != "Jaz" && strings.Contains(instructions, "Speak as Jaz,")) {
							t.Errorf("voice instructions do not identify as %q", want)
						}
						for _, kept := range []string{chatContext, "Voice model: ", "Jaz desktop app", "Jaz side browser", "Do not answer from these instructions: say you're checking, delegate the question"} {
							if !strings.Contains(instructions, kept) {
								t.Errorf("lost voice context or policy: %q", kept)
							}
						}
						if strings.Contains(instructions, "Private agent instructions.") {
							t.Error("private backend agent prompt leaked into voice instructions")
						}
					}
					if name == "ordinary" {
						connect("Jaz")
						return
					}
					connect(name)
					if err := store.UpdateSessionTitle(session.ID, "Renamed "+name); err != nil {
						t.Fatal(err)
					}
					connect("Renamed " + name)
				})
			}
		})
	}
}

func TestConnectRejectsMissingThreadBeforeResolvingCredentials(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := live.NewService(store, func(string) (live.Credential, error) {
		t.Fatal("missing thread reached provider credentials")
		return live.Credential{}, nil
	})
	handler := NewHandler(service, nil)
	response := httptest.NewRecorder()
	handler.Connect(response, httptest.NewRequest(http.MethodPost, "/v1/voice/connect", strings.NewReader(`{"sdp":"v=0\r\noffer","session_id":"missing"}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing thread = %d: %s", response.Code, response.Body.String())
	}
}
