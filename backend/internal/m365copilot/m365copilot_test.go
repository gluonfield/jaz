package m365copilot

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type memoryTokenStore struct {
	mu     sync.Mutex
	tokens map[string]integrationoauth.Token
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{tokens: map[string]integrationoauth.Token{}}
}

func (s *memoryTokenStore) LoadToken(_ context.Context, id string) (integrationoauth.Token, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, ok := s.tokens[id]
	return tok, ok, nil
}

func (s *memoryTokenStore) SaveToken(_ context.Context, id string, token integrationoauth.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[id] = token
	return nil
}

func (s *memoryTokenStore) DeleteToken(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, id)
	return nil
}

func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(body) + "."
}

func TestOAuthConfigValidation(t *testing.T) {
	if err := (OAuthConfig{}).Validate(); err == nil || !strings.Contains(err.Error(), "JAZ_MICROSOFT_COPILOT_OAUTH_CLIENT_ID") {
		t.Fatalf("missing client id error = %v", err)
	}
	for _, tenant := range []string{"consumers", "common", PersonalAccountTenantID, "Consumers"} {
		err := (OAuthConfig{ClientID: "app-client", TenantID: tenant}).Validate()
		if err == nil || !strings.Contains(err.Error(), "work or school") {
			t.Fatalf("tenant %q error = %v, want personal-account rejection", tenant, err)
		}
	}
	cfg := (OAuthConfig{ClientID: " app-client "}).Normalize()
	if cfg.TenantID != DefaultTenantID {
		t.Fatalf("default tenant = %q, want %q", cfg.TenantID, DefaultTenantID)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestOAuthPKCEFlowRefreshRejectsPersonalAccountAndDisconnects(t *testing.T) {
	var (
		mu              sync.Mutex
		nextTenantID    = "11111111-2222-3333-4444-555555555555"
		lastChallenge   string
		tokenExchanges  int
		refreshRequests int
	)

	entra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/oauth2/v2.0/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Form.Get("client_id") != "entra-client-1" {
			http.Error(w, "wrong client_id", http.StatusBadRequest)
			return
		}
		mu.Lock()
		tid := nextTenantID
		challenge := lastChallenge
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			verifier := r.Form.Get("code_verifier")
			sum := sha256.Sum256([]byte(verifier))
			gotChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
			if gotChallenge != challenge {
				http.Error(w, "pkce mismatch", http.StatusBadRequest)
				return
			}
			mu.Lock()
			tokenExchanges++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token_type":    "Bearer",
				"access_token":  "access-initial",
				"refresh_token": "refresh-initial",
				"expires_in":    1,
				"id_token":      fakeJWT(t, map[string]any{"tid": tid, "iss": "https://login.microsoftonline.com/" + tid + "/v2.0"}),
			})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-initial" {
				http.Error(w, "unexpected refresh token", http.StatusBadRequest)
				return
			}
			mu.Lock()
			refreshRequests++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token_type":    "Bearer",
				"access_token":  "access-refreshed",
				"refresh_token": "refresh-rotated",
				"expires_in":    3600,
			})
		default:
			http.Error(w, "unsupported grant", http.StatusBadRequest)
		}
	}))
	defer entra.Close()

	store := newMemoryTokenStore()
	auth := NewAuthService(store, OAuthConfig{
		ClientID:         "entra-client-1",
		AuthorityBaseURL: entra.URL,
	}, oauthbroker.Broker{})

	st := auth.Status(t.Context())
	if st.Authenticated || !st.LoginAvailable {
		t.Fatalf("initial status = %#v", st)
	}

	// Personal MSA account must be rejected before storing credentials.
	mu.Lock()
	nextTenantID = PersonalAccountTenantID
	mu.Unlock()
	personalSession, err := auth.StartLogin(t.Context(), "http://127.0.0.1:5299"+OAuthCallbackPath)
	if err != nil {
		t.Fatal(err)
	}
	parsedAuthURL, err := url.Parse(personalSession.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsedAuthURL.Path != "/organizations/oauth2/v2.0/authorize" {
		t.Fatalf("authorize path = %q", parsedAuthURL.Path)
	}
	if parsedAuthURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("expected S256 PKCE: %s", personalSession.AuthURL)
	}
	scopes := strings.Fields(parsedAuthURL.Query().Get("scope"))
	for _, want := range []string{"offline_access", "Sites.Read.All", "Mail.Read", "People.Read.All", "OnlineMeetingTranscript.Read.All", "Chat.Read", "ChannelMessage.Read.All", "ExternalItem.Read.All"} {
		found := false
		for _, got := range scopes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing required Copilot Chat API scope %q in %v", want, scopes)
		}
	}
	mu.Lock()
	lastChallenge = parsedAuthURL.Query().Get("code_challenge")
	mu.Unlock()
	err = auth.CompleteCallback(t.Context(), personalSession.State, "code-msa", "", "")
	if err == nil || !strings.Contains(err.Error(), "personal Microsoft accounts") {
		t.Fatalf("expected personal account rejection, got %v", err)
	}
	if auth.Status(t.Context()).Authenticated {
		t.Fatal("personal Microsoft account must not be persisted as authenticated")
	}

	// Work/school tenant succeeds via pasted callback URL input too.
	mu.Lock()
	nextTenantID = "11111111-2222-3333-4444-555555555555"
	mu.Unlock()
	workSession, err := auth.StartLogin(t.Context(), "http://127.0.0.1:5299"+OAuthCallbackPath)
	if err != nil {
		t.Fatal(err)
	}
	workURL, _ := url.Parse(workSession.AuthURL)
	mu.Lock()
	lastChallenge = workURL.Query().Get("code_challenge")
	mu.Unlock()

	if err := auth.CompleteFromInput(t.Context(), workSession.State, "http://127.0.0.1:5299/cb?code=code-work&state=wrong-state"); err == nil {
		t.Fatal("expected mismatched state in pasted callback URL to fail")
	}
	callbackPaste := fmt.Sprintf("http://127.0.0.1:5299%s?code=code-work&state=%s", OAuthCallbackPath, workSession.State)
	if err := auth.CompleteFromInput(t.Context(), workSession.State, callbackPaste); err != nil {
		t.Fatalf("CompleteFromInput failed: %v", err)
	}
	if err := <-workSession.Done; err != nil {
		t.Fatalf("session.Done = %v", err)
	}
	if !auth.Status(t.Context()).Authenticated {
		t.Fatal("expected authenticated status after work/school sign-in")
	}

	// Stored access token expires in 1s (inside oauth2 10s expiry delta), so TokenSource refreshes and persists rotated token.
	ts, err := auth.TokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := ts.Token()
	if err != nil {
		t.Fatalf("token refresh failed: %v", err)
	}
	if refreshed.AccessToken != "access-refreshed" {
		t.Fatalf("refreshed access token = %q, want access-refreshed", refreshed.AccessToken)
	}
	stored, ok, _ := store.LoadToken(t.Context(), TokenConnectionID)
	if !ok || stored.RefreshToken != "refresh-rotated" {
		t.Fatalf("rotated refresh token not persisted: %#v", stored)
	}
	if refreshRequests != 1 || tokenExchanges != 2 {
		t.Fatalf("tokenExchanges=%d refreshRequests=%d", tokenExchanges, refreshRequests)
	}

	if err := auth.Disconnect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if auth.Status(t.Context()).Authenticated {
		t.Fatal("expected unauthenticated after disconnect")
	}
}

func TestACPBridgeStreamsMultiTurnResumesCancelsAndValidatesInputs(t *testing.T) {
	var (
		mu             sync.Mutex
		createCalls    int
		streamCalls    int
		lastChatReq    ChatRequest
		streamBlocked  = make(chan struct{})
		streamCanceled = make(chan struct{})
		mode           = "normal"
	)

	largePadding := strings.Repeat("a", 70*1024)
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer work-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"InvalidAuthenticationToken","message":"token expired"}}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/beta/copilot/conversations":
			mu.Lock()
			createCalls++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"conv-123","status":"active","turnCount":0}`))
		case r.Method == http.MethodPost && r.URL.Path == "/beta/copilot/conversations/conv-123/chatOverStream":
			var req ChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			streamCalls++
			callNum := streamCalls
			lastChatReq = req
			currentMode := mode
			mu.Unlock()

			switch currentMode {
			case "forbidden":
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"Forbidden","message":"Missing Copilot license"}}`))
				return
			case "rai":
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"id\":\"conv-123\",\"state\":\"disengagedForRai\",\"messages\":[]}\n\n"))
				return
			case "stream_error":
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"error\":{\"code\":\"ServiceUnavailable\",\"message\":\"Copilot backend overloaded\"}}\n\n"))
				return
			case "stream_auth_error":
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"error\":{\"code\":\"InvalidAuthenticationToken\",\"message\":\"Token expired\"}}\n\n"))
				return
			case "block":
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				close(streamBlocked)
				<-r.Context().Done()
				close(streamCanceled)
				return
			}

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			writeEvent := func(payload any) {
				b, _ := json.Marshal(payload)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
				if flusher != nil {
					flusher.Flush()
				}
			}
			if callNum == 1 {
				writeEvent(Conversation{
					ID: "conv-123", State: "active", TurnCount: 1,
					Messages: []ConversationMessage{
						{ODataType: odataTypeResponseMessage, ID: "resp-1", Text: "Hello "},
					},
				})
				writeEvent(Conversation{
					ID: "conv-123", State: "active", TurnCount: 1,
					Messages: []ConversationMessage{
						{
							ODataType: odataTypeResponseMessage,
							ID:        "resp-1",
							Text:      "Hello world " + largePadding,
							Attributions: []MessageAttribution{
								{ProviderDisplayName: "Contoso Spec", SeeMoreWebURL: "https://contoso.sharepoint.com/spec.docx"},
								{ProviderDisplayName: "Contoso Spec", SeeMoreWebURL: "https://contoso.sharepoint.com/spec.docx"},
							},
						},
					},
				})
				return
			}
			// Turn 2 returns full conversation history including turn 1 request & response; bridge must emit only resp-2.
			writeEvent(Conversation{
				ID: "conv-123", State: "active", TurnCount: 2,
				Messages: []ConversationMessage{
					{ODataType: odataTypeRequestMessage, ID: "req-1", Text: "turn 1"},
					{ODataType: odataTypeResponseMessage, ID: "resp-1", Text: "Hello world " + largePadding},
					{ODataType: odataTypeRequestMessage, ID: "req-2", Text: req.Message.Text},
				},
			})
			writeEvent(Conversation{
				ID: "conv-123", State: "active", TurnCount: 2,
				Messages: []ConversationMessage{
					{ODataType: odataTypeRequestMessage, ID: "req-1", Text: "turn 1"},
					{ODataType: odataTypeResponseMessage, ID: "resp-1", Text: "Hello world " + largePadding},
					{ODataType: odataTypeRequestMessage, ID: "req-2", Text: req.Message.Text},
					{ODataType: odataTypeResponseMessage, ID: "resp-2", Text: "Follow-up answer"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer graph.Close()

	store := newMemoryTokenStore()
	_ = store.SaveToken(t.Context(), TokenConnectionID, integrationoauth.Token{
		AccessToken: "work-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
		ClientID:    "entra-client-1",
	})
	auth := NewAuthService(store, OAuthConfig{
		ClientID:     "entra-client-1",
		GraphBaseURL: graph.URL + "/beta",
	}, oauthbroker.Broker{})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var (
		chunksMu sync.Mutex
		chunks   []string
	)
	conn := OpenACPConn(ctx, auth)
	client := jsonrpc.NewPeer(conn, jsonrpc.HandlerFunc(func(_ context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		if req.Method == acpschema.ClientMethodSessionUpdate {
			var note struct {
				Update json.RawMessage `json:"update"`
			}
			_ = json.Unmarshal(req.Params, &note)
			if decoded, err := acpschema.DecodeSessionUpdate(note.Update); err == nil {
				if chunk, ok := decoded.(acpschema.AgentMessageChunkUpdate); ok {
					if blk, err := acpschema.DecodeContentBlock(chunk.Content); err == nil {
						if txt, ok := blk.(acpschema.TextContentBlock); ok {
							chunksMu.Lock()
							chunks = append(chunks, txt.Text)
							chunksMu.Unlock()
						}
					}
				}
			}
		}
		return jsonrpc.EncodeResult(map[string]any{})
	}))
	go func() { _ = client.Serve(ctx) }()
	defer client.Close()

	initRaw, err := client.Call(ctx, acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
	})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	var initResp acpschema.InitializeResponse
	if err := json.Unmarshal(initRaw, &initResp); err != nil {
		t.Fatal(err)
	}
	if initResp.AgentInfo == nil || initResp.AgentInfo.Title != DisplayName {
		t.Fatalf("AgentInfo = %#v, want title %q", initResp.AgentInfo, DisplayName)
	}
	if initResp.AgentCapabilities == nil || initResp.AgentCapabilities.SessionCapabilities == nil || initResp.AgentCapabilities.SessionCapabilities.Resume == nil {
		t.Fatalf("missing truthful resume capability: %#v", initResp.AgentCapabilities)
	}
	if initResp.AgentCapabilities.MCPCapabilities != nil && (initResp.AgentCapabilities.MCPCapabilities.HTTP || initResp.AgentCapabilities.MCPCapabilities.SSE) {
		t.Fatalf("unexpected MCP capability advertised: %#v", initResp.AgentCapabilities.MCPCapabilities)
	}

	newRaw, err := client.Call(ctx, acpschema.AgentMethodSessionNew, map[string]any{"cwd": "/tmp", "mcpServers": []any{}})
	if err != nil {
		t.Fatalf("session/new failed: %v", err)
	}
	var newResp acpschema.NewSessionResponse
	if err := json.Unmarshal(newRaw, &newResp); err != nil {
		t.Fatal(err)
	}
	if newResp.SessionID != "conv-123" || newResp.Modes != nil || len(newResp.ConfigOptions) != 0 || strings.Contains(string(newRaw), `"models"`) {
		t.Fatalf("unexpected session/new response: %#v (%s)", newResp, string(newRaw))
	}

	textBlock, _ := json.Marshal(acpschema.TextContentBlock{Kind: acpschema.ContentBlockText, TextContent: acpschema.TextContent{Text: "Summarize spec"}})
	spBlock, _ := json.Marshal(acpschema.ResourceLinkContentBlock{
		Kind:         acpschema.ContentBlockResourceLink,
		ResourceLink: acpschema.ResourceLink{Name: "spec.docx", URI: "https://contoso.sharepoint.com/spec.docx"},
	})
	promptRaw, err := client.Call(ctx, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{
		SessionID: newResp.SessionID,
		Prompt:    []acpschema.ContentBlock{textBlock, spBlock},
	})
	if err != nil {
		t.Fatalf("session/prompt turn 1 failed: %v", err)
	}
	var promptResp acpschema.PromptResponse
	_ = json.Unmarshal(promptRaw, &promptResp)
	if promptResp.StopReason != acpschema.StopReasonEndTurn {
		t.Fatalf("stop reason = %q, want end_turn", promptResp.StopReason)
	}
	chunksMu.Lock()
	gotTurn1 := strings.Join(chunks, "")
	chunks = nil
	chunksMu.Unlock()
	if !strings.HasPrefix(gotTurn1, "Hello world "+largePadding) || !strings.Contains(gotTurn1, "[Contoso Spec](<https://contoso.sharepoint.com/spec.docx>)") {
		t.Fatalf("turn 1 streamed output len=%d did not preserve large delta and deduplicated attribution", len(gotTurn1))
	}
	if strings.Count(gotTurn1, "https://contoso.sharepoint.com/spec.docx") != 1 {
		t.Fatalf("attribution URL was not deduplicated")
	}
	mu.Lock()
	if lastChatReq.LocationHint.TimeZone == "" || lastChatReq.ContextualResources == nil || len(lastChatReq.ContextualResources.Files) != 1 {
		t.Fatalf("unexpected chat request: %#v", lastChatReq)
	}
	mu.Unlock()

	// Reject local file:// attachments truthfully instead of silently dropping them.
	localFileBlock, _ := json.Marshal(acpschema.ResourceLinkContentBlock{
		Kind:         acpschema.ContentBlockResourceLink,
		ResourceLink: acpschema.ResourceLink{Name: "secret.txt", URI: "file:///tmp/secret.txt"},
	})
	if _, err := client.Call(ctx, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{
		SessionID: newResp.SessionID,
		Prompt:    []acpschema.ContentBlock{textBlock, localFileBlock},
	}); err == nil || !strings.Contains(err.Error(), "SharePoint/OneDrive") {
		t.Fatalf("expected truthful local attachment rejection, got %v", err)
	}

	// Resume on a brand new bridge connection and confirm full-history turn 2 does NOT replay turn 1.
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	var turn2Chunks []string
	var turn2Mu sync.Mutex
	conn2 := OpenACPConn(ctx2, auth)
	client2 := jsonrpc.NewPeer(conn2, jsonrpc.HandlerFunc(func(_ context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		if req.Method == acpschema.ClientMethodSessionUpdate {
			var note struct {
				Update json.RawMessage `json:"update"`
			}
			_ = json.Unmarshal(req.Params, &note)
			if decoded, err := acpschema.DecodeSessionUpdate(note.Update); err == nil {
				if chunk, ok := decoded.(acpschema.AgentMessageChunkUpdate); ok {
					if blk, err := acpschema.DecodeContentBlock(chunk.Content); err == nil {
						if txt, ok := blk.(acpschema.TextContentBlock); ok {
							turn2Mu.Lock()
							turn2Chunks = append(turn2Chunks, txt.Text)
							turn2Mu.Unlock()
						}
					}
				}
			}
		}
		return jsonrpc.EncodeResult(map[string]any{})
	}))
	go func() { _ = client2.Serve(ctx2) }()
	defer client2.Close()

	if _, err := client2.Call(ctx2, acpschema.AgentMethodSessionResume, map[string]any{"sessionId": "conv-123", "cwd": "/tmp", "mcpServers": []any{}}); err != nil {
		t.Fatalf("session/resume failed: %v", err)
	}
	followBlock, _ := json.Marshal(acpschema.TextContentBlock{Kind: acpschema.ContentBlockText, TextContent: acpschema.TextContent{Text: "What next?"}})
	if _, err := client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{
		SessionID: "conv-123",
		Prompt:    []acpschema.ContentBlock{followBlock},
	}); err != nil {
		t.Fatalf("resumed prompt failed: %v", err)
	}
	turn2Mu.Lock()
	gotTurn2 := strings.Join(turn2Chunks, "")
	turn2Mu.Unlock()
	if gotTurn2 != "Follow-up answer" {
		t.Fatalf("resumed turn 2 = %q, want only %q", gotTurn2, "Follow-up answer")
	}

	// Cancel cancels the in-flight HTTP SSE stream and returns StopReasonCancelled.
	mu.Lock()
	mode = "block"
	mu.Unlock()
	promptDone := make(chan acpschema.PromptResponse, 1)
	go func() {
		raw, callErr := client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{
			SessionID: "conv-123",
			Prompt:    []acpschema.ContentBlock{followBlock},
		})
		if callErr != nil {
			t.Errorf("cancelled prompt error = %v", callErr)
			return
		}
		var r acpschema.PromptResponse
		_ = json.Unmarshal(raw, &r)
		promptDone <- r
	}()
	<-streamBlocked
	if err := client2.Notify(ctx2, acpschema.AgentMethodSessionCancel, acpschema.CancelNotification{SessionID: "conv-123"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-streamCanceled:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight HTTP stream was not cancelled")
	}
	select {
	case resp := <-promptDone:
		if resp.StopReason != acpschema.StopReasonCancelled {
			t.Fatalf("cancelled stop reason = %q", resp.StopReason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled prompt did not return")
	}

	// 403 surfaces license/permission guidance, RAI disengagement returns clear error, 401 maps to ACP ErrorCode32000.
	mu.Lock()
	mode = "forbidden"
	mu.Unlock()
	if _, err := client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{SessionID: "conv-123", Prompt: []acpschema.ContentBlock{followBlock}}); err == nil || !strings.Contains(err.Error(), "Microsoft 365 Copilot license") {
		t.Fatalf("403 error = %v", err)
	}

	mu.Lock()
	mode = "rai"
	mu.Unlock()
	if _, err := client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{SessionID: "conv-123", Prompt: []acpschema.ContentBlock{followBlock}}); err == nil || !strings.Contains(err.Error(), "Responsible AI") {
		t.Fatalf("RAI error = %v", err)
	}

	mu.Lock()
	mode = "stream_error"
	mu.Unlock()
	if _, err := client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{SessionID: "conv-123", Prompt: []acpschema.ContentBlock{followBlock}}); err == nil || !strings.Contains(err.Error(), "Copilot backend overloaded") {
		t.Fatalf("in-stream Graph error = %v", err)
	}

	mu.Lock()
	mode = "stream_auth_error"
	mu.Unlock()
	_, err = client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{SessionID: "conv-123", Prompt: []acpschema.ContentBlock{followBlock}})
	var streamAuthErr *jsonrpc.Error
	if !errors.As(err, &streamAuthErr) || streamAuthErr.Code != int(acpschema.ErrorCode32000) {
		t.Fatalf("in-stream auth error = %v, want ACP auth_required (-32000)", err)
	}

	// Misconfigured personal tenant must not report a cached token as usable.
	personalCfgSvc := NewAuthService(store, OAuthConfig{
		ClientID: "entra-client-1",
		TenantID: "consumers",
	}, oauthbroker.Broker{})
	if st := personalCfgSvc.Status(t.Context()); st.Authenticated || st.LoginAvailable || !strings.Contains(st.Reason, "work or school") {
		t.Fatalf("misconfigured personal tenant status = %#v", st)
	}

	_ = store.SaveToken(t.Context(), TokenConnectionID, integrationoauth.Token{
		AccessToken: "bad-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	})
	_, err = client2.Call(ctx2, acpschema.AgentMethodSessionPrompt, acpschema.PromptRequest{SessionID: "conv-123", Prompt: []acpschema.ContentBlock{followBlock}})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != int(acpschema.ErrorCode32000) {
		t.Fatalf("401 error = %v, want ACP auth_required (-32000)", err)
	}

	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1 (resume must reuse conversation)", createCalls)
	}
}
