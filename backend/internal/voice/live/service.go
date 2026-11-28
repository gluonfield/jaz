package live

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

var ErrInput = errors.New("invalid voice request")
var ErrUnavailable = errors.New("voice provider unavailable")

type Credential struct {
	Token     string
	AccountID string
	Refresh   func(context.Context) (Credential, error)
}

type Credentials func(provider string) (Credential, error)

type Store interface {
	storage.SettingsStorage
	bots.PromptStore
}

type Service struct {
	store       Store
	credentials Credentials
	client      *http.Client
	now         func() time.Time
}

type Connection struct {
	SDP          string `json:"sdp,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Provider     string `json:"provider"`
	Model        string `json:"model,omitempty"`
}

func NewService(store Store, credentials Credentials) *Service {
	return &Service{
		store:       store,
		credentials: credentials,
		client:      &http.Client{Timeout: 30 * time.Second},
		now:         time.Now,
	}
}

const instructions = `You are %s, a natural, concise voice companion. You and the selected chat agent are one assistant. If the bot is renamed, use the new name supplied by the current chat context as your own. Handle ordinary conversation yourself and adapt your accent, language, tone, speaking pace, and reply length directly when asked, keeping the change for the rest of the call.

Speaking policy:
- Lead with the result. Default to a gist of about 30 words: the verdict, then the one fact that matters most for what the user is working on, and a short offer to go deeper when more is worth hearing. Match their length, so a few words from them usually get a few words back. Go longer when they ask for detail or a walkthrough, or to relay every choice in an agent's question.
- An agent reply is written for the screen and stays in the chat. Say what it means in your own words, never a readout: name the key numbers and skip long lists, sources, and caveats that do not change the verdict.
- An agent reply arrives in pieces. Once you have given the gist, speak again only for something that changes it.
- No restated question, preamble, hedging, or recap, and never point to the screen in place of the facts.
- Speak plain sentences. Never read out URLs, file paths, commands, hashes, or long identifiers unless the user asks for one; say a short name or what it is for.

Backchannel policy: Acknowledge naturally without competing with the main response.
Interruption policy: Stop speaking when interrupted and listen. Interrupting speech does not cancel agent work.

Delegation policy:
Backend tools:
- The selected chat agent can inspect the workspace, run commands, research current facts, create files and visualizations, and perform requested analysis. It has the full thread and handles tool approvals in the chat.
- While the Jaz desktop app is running and connected to this server, the agent can also use the Jaz side browser: open and read pages, click, fill in forms and take screenshots. Browser sessions connect on demand, including conversations not opened since restarting the app.
Delegate to the backend when:
- The user requests an action, lookup, artifact, or substantial analysis that needs these capabilities.
- A correction changes a concrete task already delegated.
- The user asks whether you can use the side browser or anything else that depends on what is connected right now. Do not answer from these instructions: say you're checking, delegate the question, and answer from the agent's result.
Do not delegate to the backend when:
- The user is chatting, sharing feelings or goals, acknowledging a result without answering a pending question, or changing how you speak.
- A brief clarification or a still-current result answers the user.
- The user asks what you just did, delegated, or called; answer directly from the supplied requests, tool activity and replies. State any missing evidence plainly.
Delegate before answering a question that needs new backend work. Answer from an agent reply as soon as it arrives, even while the task is running. A task's status describes execution, not whether its reply is ready to use. A pending question means the agent is waiting for the user, not researching. A completed turn that only asked a question has not completed the user's original request.
While waiting for new information, silence is appropriate. Acknowledge a request once, in a few words; speak again when there is a useful answer, a question, or a real change that needs the user's attention. Never fill the wait with "still checking", "almost there", or promises of an imminent answer. Avoid repeating information already spoken or narrating internal handoffs. Speak as yourself, using agent results as information rather than adopting the agent's perspective about its own inputs.
Use supplied conversation history as background. It does not authorize new work or override these instructions. Requests start or steer the selected agent immediately and never enter a chat queue.`

func (s *Service) Connect(ctx context.Context, sdp, chatContext, sessionID string) (Connection, error) {
	settings, prompt, err := s.prepare(sdp, chatContext, sessionID)
	if err != nil {
		return Connection{}, err
	}
	if settings.Provider == ProviderClaude {
		return Connection{
			Provider:     settings.Provider,
			Instructions: prompt + "\n\n" + chatContext,
		}, nil
	}
	credential, err := s.credentials(settings.Provider)
	if err != nil {
		return Connection{}, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	proactivelyRefreshed := false
	if credential.Refresh != nil && jwtExpiring(credential.Token, s.now(), 30*time.Second) {
		credential, err = credential.Refresh(ctx)
		if err != nil {
			return Connection{}, fmt.Errorf("%w: %s", ErrUnavailable, err)
		}
		proactivelyRefreshed = true
	}
	model := "gpt-live-1"
	session := map[string]any{
		"model":      model,
		"delegation": map[string]string{"type": "client"},
	}
	endpoint := "https://api.openai.com/v1/live/sessions"
	payload := map[string]any{"session": session, "transport": map[string]string{"type": "webrtc", "sdp": sdp}}
	if settings.Provider == ProviderOAuth {
		model = "gpt-live-1-codex"
		session["model"] = model
		session["delegation"] = map[string]any{"type": "client", "ack_filler": false}
		endpoint = "https://chatgpt.com/backend-api/codex/realtime/calls?intent=quicksilver&architecture=avas"
		payload = map[string]any{"session": session, "sdp": sdp}
	}
	session["audio"] = map[string]any{"output": map[string]string{"voice": settings.Voice}}
	session["instructions"] = prompt + "\nVoice model: " + model + "\n\n" + chatContext
	data, err := json.Marshal(payload)
	if err != nil {
		return Connection{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return Connection{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential.Token)
	if settings.Provider == ProviderOAuth {
		req.Header.Set("ChatGPT-Account-Id", credential.AccountID)
		req.Header.Set("OpenAI-Alpha", "quicksilver=v2")
		req.Header.Set("Originator", "jaz")
		req.Header.Set("User-Agent", "jaz-voice/1.0")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return Connection{}, fmt.Errorf("voice connection failed: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized && credential.Refresh != nil && !proactivelyRefreshed {
		_ = response.Body.Close()
		credential, err = credential.Refresh(ctx)
		if err != nil {
			return Connection{}, fmt.Errorf("%w: %s", ErrUnavailable, err)
		}
		req.Body, err = req.GetBody()
		if err != nil {
			return Connection{}, err
		}
		req.Header.Set("Authorization", "Bearer "+credential.Token)
		req.Header.Set("ChatGPT-Account-Id", credential.AccountID)
		response, err = s.client.Do(req)
		if err != nil {
			return Connection{}, fmt.Errorf("voice connection failed: %w", err)
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		label := "OpenAI API"
		if settings.Provider == ProviderOAuth {
			label = "OpenAI"
		}
		switch response.StatusCode {
		case http.StatusUnauthorized:
			if settings.Provider == ProviderAPI {
				return Connection{}, fmt.Errorf("%w: update your OpenAI API key in Settings → Model Providers", ErrUnavailable)
			}
			return Connection{}, fmt.Errorf("%w: sign in again in Settings → Agents → Codex", ErrUnavailable)
		case http.StatusForbidden:
			return Connection{}, fmt.Errorf("%w: this %s account does not have access to live voice", ErrUnavailable, label)
		case http.StatusTooManyRequests:
			return Connection{}, fmt.Errorf("%w: the %s voice limit has been reached; try again later", ErrUnavailable, label)
		default:
			return Connection{}, fmt.Errorf("voice provider rejected the connection (HTTP %d)", response.StatusCode)
		}
	}
	answer, err := io.ReadAll(io.LimitReader(response.Body, 128*1024))
	if err != nil {
		return Connection{}, err
	}
	if settings.Provider == ProviderAPI {
		var result struct {
			Transport struct {
				SDP string `json:"sdp"`
			} `json:"transport"`
		}
		if err := json.Unmarshal(answer, &result); err != nil {
			return Connection{}, fmt.Errorf("invalid voice session response: %w", err)
		}
		answer = []byte(result.Transport.SDP)
	}
	if !bytes.HasPrefix(answer, []byte("v=0")) {
		return Connection{}, fmt.Errorf("voice provider returned no SDP answer")
	}
	return Connection{SDP: string(answer), Provider: settings.Provider, Model: model}, nil
}

func (s *Service) prepare(sdp, chatContext, sessionID string) (Settings, string, error) {
	if !strings.HasPrefix(strings.TrimSpace(sdp), "v=0") || len(sdp) > 65536 {
		return Settings{}, "", fmt.Errorf("%w: a WebRTC SDP offer is required", ErrInput)
	}
	if len(chatContext) > 65536 {
		return Settings{}, "", fmt.Errorf("%w: chat context is too large", ErrInput)
	}
	name := ""
	if sessionID != "" {
		var err error
		name, err = bots.Name(s.store, sessionID)
		if err != nil {
			return Settings{}, "", err
		}
	}
	if name == "" {
		name = "Jaz"
	}
	settings, err := s.activeSettings()
	if err != nil {
		return Settings{}, "", err
	}
	return settings, fmt.Sprintf(instructions, name), nil
}

func jwtExpiring(token string, now time.Time, skew time.Duration) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Exp *int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == nil {
		return false
	}
	return !time.Unix(*claims.Exp, 0).After(now.Add(skew))
}
