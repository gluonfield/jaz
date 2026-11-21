package m365copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

const (
	odataTypeRequestMessage  = "#microsoft.graph.copilotConversationRequestMessage"
	odataTypeResponseMessage = "#microsoft.graph.copilotConversationResponseMessage"
	maxGraphErrorBytes       = 16 * 1024
)

type AuthError struct {
	Err error
}

func (e *AuthError) Error() string {
	return e.Err.Error()
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

type Conversation struct {
	ID              string                `json:"id"`
	CreatedDateTime string                `json:"createdDateTime,omitempty"`
	DisplayName     string                `json:"displayName,omitempty"`
	State           string                `json:"state,omitempty"`
	Status          string                `json:"status,omitempty"`
	TurnCount       int                   `json:"turnCount,omitempty"`
	Messages        []ConversationMessage `json:"messages,omitempty"`
}

func (c Conversation) DisengagedForRAI() bool {
	return strings.EqualFold(c.State, "disengagedForRai") || strings.EqualFold(c.Status, "disengagedForRai")
}

type ConversationMessage struct {
	ODataType    string               `json:"@odata.type,omitempty"`
	ID           string               `json:"id,omitempty"`
	Text         string               `json:"text,omitempty"`
	Attributions []MessageAttribution `json:"attributions,omitempty"`
}

func (m ConversationMessage) IsRequest() bool {
	return strings.EqualFold(m.ODataType, odataTypeRequestMessage)
}

func (m ConversationMessage) IsResponse() bool {
	if m.ODataType == "" {
		return true
	}
	return strings.EqualFold(m.ODataType, odataTypeResponseMessage)
}

type MessageAttribution struct {
	AttributionType     string `json:"attributionType,omitempty"`
	ProviderDisplayName string `json:"providerDisplayName,omitempty"`
	AttributionSource   string `json:"attributionSource,omitempty"`
	SeeMoreWebURL       string `json:"seeMoreWebUrl,omitempty"`
}

type ChatRequest struct {
	Message             RequestMessage       `json:"message"`
	AdditionalContext   []ContextMessage     `json:"additionalContext,omitempty"`
	LocationHint        LocationHint         `json:"locationHint"`
	ContextualResources *ContextualResources `json:"contextualResources,omitempty"`
}

type RequestMessage struct {
	Text string `json:"text"`
}

type ContextMessage struct {
	Text string `json:"text"`
}

type LocationHint struct {
	TimeZone string `json:"timeZone"`
}

type ContextualResources struct {
	Files      []ContextFile `json:"files,omitempty"`
	WebContext *WebContext   `json:"webContext,omitempty"`
}

type ContextFile struct {
	URI string `json:"uri"`
}

type WebContext struct {
	IsWebEnabled bool `json:"isWebEnabled"`
}

type Client struct {
	auth     *AuthService
	timeZone func() string
}

func NewClient(auth *AuthService) *Client {
	return &Client{
		auth:     auth,
		timeZone: LocalIANATimeZone,
	}
}

func (c *Client) bearerToken(ctx context.Context) (string, error) {
	source, err := c.auth.TokenSource(ctx)
	if err != nil {
		if errors.Is(err, integrationoauth.ErrTokenNotFound) {
			return "", &AuthError{Err: errors.New("Microsoft Work/School account is not signed in")}
		}
		return "", err
	}
	token, err := source.Token()
	if err != nil {
		var refreshErr *integrationoauth.RefreshError
		if errors.As(err, &refreshErr) {
			return "", &AuthError{Err: fmt.Errorf("refresh Microsoft Work/School token: %w", err)}
		}
		return "", err
	}
	if token.AccessToken == "" {
		return "", &AuthError{Err: errors.New("Microsoft Work/School access token is empty")}
	}
	return token.AccessToken, nil
}

func (c *Client) CreateConversation(ctx context.Context) (Conversation, error) {
	token, err := c.bearerToken(ctx)
	if err != nil {
		return Conversation{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.auth.GraphBaseURL()+"/copilot/conversations", bytes.NewReader([]byte("{}")))
	if err != nil {
		return Conversation{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.auth.HTTPClient().Do(req)
	if err != nil {
		return Conversation{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxGraphErrorBytes))
		return Conversation{}, parseGraphError(resp.StatusCode, body)
	}
	var conv Conversation
	if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
		return Conversation{}, fmt.Errorf("decode Microsoft 365 Copilot conversation: %w", err)
	}
	if conv.ID == "" {
		return Conversation{}, errors.New("Microsoft 365 Copilot API returned an empty conversation ID")
	}
	return conv, nil
}

func (c *Client) ChatStream(ctx context.Context, conversationID string, input ChatRequest, onUpdate func(Conversation) error) (Conversation, error) {
	token, err := c.bearerToken(ctx)
	if err != nil {
		return Conversation{}, err
	}
	if input.LocationHint.TimeZone == "" {
		input.LocationHint.TimeZone = c.timeZone()
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return Conversation{}, err
	}
	endpoint := c.auth.GraphBaseURL() + "/copilot/conversations/" + url.PathEscape(conversationID) + "/chatOverStream"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Conversation{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.auth.HTTPClient().Do(req)
	if err != nil {
		return Conversation{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxGraphErrorBytes))
		return Conversation{}, parseGraphError(resp.StatusCode, body)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var conv Conversation
		if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
			return Conversation{}, fmt.Errorf("decode Microsoft 365 Copilot response: %w", err)
		}
		if onUpdate != nil {
			if err := onUpdate(conv); err != nil {
				return conv, err
			}
		}
		return conv, nil
	}

	return readSSEConversationStream(resp.Body, onUpdate)
}

func readSSEConversationStream(body io.Reader, onUpdate func(Conversation) error) (Conversation, error) {
	reader := bufio.NewReader(body)
	var (
		dataLines     []string
		latest        Conversation
		receivedEvent bool
	)
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		raw := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if raw == "" || raw == "[DONE]" {
			return nil
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return fmt.Errorf("decode Microsoft 365 Copilot stream event: %w", err)
		}
		if event.Error != nil && (event.Error.Code != "" || event.Error.Message != "") {
			return formatStreamGraphError(event.Error.Code, event.Error.Message)
		}
		receivedEvent = true
		latest = event.Conversation
		if onUpdate != nil {
			return onUpdate(event.Conversation)
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return latest, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if flushErr := flush(); flushErr != nil {
				return latest, flushErr
			}
		} else if after, ok := strings.CutPrefix(trimmed, "data:"); ok {
			dataLines = append(dataLines, strings.TrimPrefix(after, " "))
		}
		if errors.Is(err, io.EOF) {
			if flushErr := flush(); flushErr != nil {
				return latest, flushErr
			}
			if !receivedEvent {
				return latest, errors.New("Microsoft 365 Copilot stream closed before returning a conversation event")
			}
			return latest, nil
		}
	}
}

type graphErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type graphErrorEnvelope struct {
	Error graphErrorDetail `json:"error"`
}

type streamEvent struct {
	Conversation
	Error *graphErrorDetail `json:"error,omitempty"`
}

func formatStreamGraphError(code, message string) error {
	status := http.StatusBadGateway
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "invalidauthenticationtoken", "unauthenticated", "unauthorized":
		status = http.StatusUnauthorized
	case "forbidden", "accessdenied":
		status = http.StatusForbidden
	}
	return formatGraphError(status, code, message)
}

func parseGraphError(status int, body []byte) error {
	var env graphErrorEnvelope
	if json.Unmarshal(body, &env) == nil && (env.Error.Code != "" || env.Error.Message != "") {
		return formatGraphError(status, env.Error.Code, env.Error.Message)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = http.StatusText(status)
	}
	return formatGraphError(status, "", text)
}

func formatGraphError(status int, code, message string) error {
	detail := strings.TrimSpace(message)
	if code != "" && detail != "" {
		detail = code + ": " + detail
	} else if code != "" {
		detail = code
	}
	if detail == "" {
		detail = http.StatusText(status)
	}
	switch status {
	case http.StatusUnauthorized:
		return &AuthError{Err: fmt.Errorf("Microsoft Graph rejected credentials (401): %s", detail)}
	case http.StatusForbidden:
		return fmt.Errorf("Microsoft 365 Copilot API denied access (403): %s (confirm the signed-in work/school account has a Microsoft 365 Copilot license and delegated Graph permissions are granted)", detail)
	case http.StatusNotFound:
		return fmt.Errorf("Microsoft 365 Copilot conversation was not found (404): %s", detail)
	default:
		return fmt.Errorf("Microsoft 365 Copilot API failed (%d): %s", status, detail)
	}
}

func LocalIANATimeZone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" && tz != "Local" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if name := time.Local.String(); name != "" && name != "Local" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if _, after, ok := strings.Cut(filepath.ToSlash(target), "zoneinfo/"); ok && after != "" {
			if _, err := time.LoadLocation(after); err == nil {
				return after
			}
		}
	}
	return "UTC"
}
