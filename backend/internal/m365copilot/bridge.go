package m365copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

type acpBridge struct {
	client *Client
	peer   *jsonrpc.Peer

	mu       sync.Mutex
	sessions map[string]*bridgeSession
}

type bridgeSession struct {
	seenResponseIDs map[string]struct{}
	activeCancel    context.CancelFunc
	cancelled       bool
}

func OpenACPConn(ctx context.Context, auth *AuthService) jsonrpc.MessageConn {
	clientConn, serverConn := jsonrpc.Pipe(64)
	bridge := &acpBridge{
		client:   NewClient(auth),
		sessions: map[string]*bridgeSession{},
	}
	peer := jsonrpc.NewPeer(serverConn, bridge)
	bridge.peer = peer
	go func() {
		defer serverConn.Close()
		_ = peer.Serve(ctx)
	}()
	return clientConn
}

func (b *acpBridge) HandleJSONRPC(ctx context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
	switch req.Method {
	case acpschema.AgentMethodInitialize:
		return b.handleInitialize(req.Params)
	case acpschema.AgentMethodSessionNew:
		return b.handleSessionNew(ctx)
	case acpschema.AgentMethodSessionResume:
		return b.handleSessionResume(ctx, req.Params)
	case acpschema.AgentMethodSessionPrompt:
		return b.handleSessionPrompt(ctx, req.Params)
	case acpschema.AgentMethodSessionCancel:
		return b.handleSessionCancel(req.Params)
	default:
		return nil, jsonrpc.MethodNotFound(req.Method)
	}
}

func (b *acpBridge) handleInitialize(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	req, rpcErr := jsonrpc.DecodeParams[acpschema.InitializeRequest](raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if req.ProtocolVersion != acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber) {
		return nil, jsonrpc.InvalidParams(fmt.Sprintf("unsupported ACP protocol version %d", req.ProtocolVersion), nil)
	}
	return jsonrpc.EncodeResult(acpschema.InitializeResponse{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
		AgentInfo: &acpschema.Implementation{
			Name:    AgentName,
			Title:   DisplayName,
			Version: "0.1.0",
		},
		AgentCapabilities: &acpschema.AgentCapabilities{
			LoadSession: false,
			SessionCapabilities: &acpschema.SessionCapabilities{
				Resume: &acpschema.SessionResumeCapabilities{},
			},
			MCPCapabilities: &acpschema.MCPCapabilities{
				HTTP: false,
				SSE:  false,
			},
			PromptCapabilities: &acpschema.PromptCapabilities{
				Audio:           false,
				EmbeddedContext: false,
				Image:           false,
			},
		},
	})
}

func (b *acpBridge) handleSessionNew(ctx context.Context) (json.RawMessage, *jsonrpc.Error) {
	conv, err := b.client.CreateConversation(ctx)
	if err != nil {
		return nil, mapBridgeError(err)
	}
	b.mu.Lock()
	b.sessions[conv.ID] = &bridgeSession{seenResponseIDs: map[string]struct{}{}}
	b.mu.Unlock()
	return jsonrpc.EncodeResult(acpschema.NewSessionResponse{
		SessionID: acpschema.SessionID(conv.ID),
	})
}

func (b *acpBridge) handleSessionResume(ctx context.Context, raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req struct {
		SessionID acpschema.SessionID `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid session/resume request", nil)
	}
	sessionID := string(req.SessionID)
	if sessionID == "" {
		return nil, jsonrpc.InvalidParams("sessionId is required", nil)
	}
	if _, err := b.client.bearerToken(ctx); err != nil {
		return nil, mapBridgeError(err)
	}
	b.mu.Lock()
	if _, ok := b.sessions[sessionID]; !ok {
		b.sessions[sessionID] = &bridgeSession{seenResponseIDs: map[string]struct{}{}}
	}
	b.mu.Unlock()
	return jsonrpc.EncodeResult(acpschema.LoadSessionResponse{})
}

func (b *acpBridge) handleSessionPrompt(ctx context.Context, raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	req, rpcErr := jsonrpc.DecodeParams[acpschema.PromptRequest](raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	sessionID := string(req.SessionID)
	if sessionID == "" {
		return nil, jsonrpc.InvalidParams("sessionId is required", nil)
	}
	chatReq, err := buildChatRequest(req.Prompt)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}

	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	session, seenSnapshot, startErr := b.beginTurn(sessionID, cancel)
	if startErr != nil {
		return nil, startErr
	}
	defer func() {
		b.endTurn(session)
	}()

	tracker := newStreamTracker(seenSnapshot)
	latest, err := b.client.ChatStream(turnCtx, sessionID, chatReq, func(conv Conversation) error {
		chunks, advanceErr := tracker.advance(conv)
		if advanceErr != nil {
			return advanceErr
		}
		for _, chunk := range chunks {
			if notifyErr := b.sendMessageChunk(turnCtx, req.SessionID, chunk.messageID, chunk.text); notifyErr != nil {
				return notifyErr
			}
		}
		return nil
	})
	wasCancelled := b.isTurnCancelled(session)
	if err != nil {
		if wasCancelled || errors.Is(err, context.Canceled) {
			return jsonrpc.EncodeResult(acpschema.PromptResponse{StopReason: acpschema.StopReasonCancelled})
		}
		return nil, mapBridgeError(err)
	}

	b.recordSeenResponses(session, tracker.turnResponseIDs)

	if wasCancelled {
		return jsonrpc.EncodeResult(acpschema.PromptResponse{StopReason: acpschema.StopReasonCancelled})
	}

	attributions := collectAttributions(currentTurnResponses(latest.Messages, seenSnapshot))
	citations := formatAttributions(attributions, tracker.allEmitted.String(), tracker.emittedAny)
	if citations != "" {
		if notifyErr := b.sendMessageChunk(turnCtx, req.SessionID, tracker.lastMessageID, citations); notifyErr != nil {
			if b.isTurnCancelled(session) || errors.Is(notifyErr, context.Canceled) {
				return jsonrpc.EncodeResult(acpschema.PromptResponse{StopReason: acpschema.StopReasonCancelled})
			}
			return nil, mapBridgeError(notifyErr)
		}
	}

	if latest.DisengagedForRAI() {
		if !tracker.emittedAny {
			return nil, jsonrpc.InternalError("Microsoft 365 Copilot disengaged this conversation under Responsible AI policy", nil)
		}
		return jsonrpc.EncodeResult(acpschema.PromptResponse{StopReason: acpschema.StopReasonRefusal})
	}
	if !tracker.emittedAny && citations == "" {
		return nil, jsonrpc.InternalError("Microsoft 365 Copilot stream ended without a response message", nil)
	}
	return jsonrpc.EncodeResult(acpschema.PromptResponse{StopReason: acpschema.StopReasonEndTurn})
}

func (b *acpBridge) handleSessionCancel(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	req, rpcErr := jsonrpc.DecodeParams[acpschema.CancelNotification](raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	sessionID := string(req.SessionID)
	b.mu.Lock()
	var cancel context.CancelFunc
	if session := b.sessions[sessionID]; session != nil && session.activeCancel != nil {
		session.cancelled = true
		cancel = session.activeCancel
	}
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return jsonrpc.EncodeResult(map[string]any{})
}

func (b *acpBridge) beginTurn(sessionID string, cancel context.CancelFunc) (*bridgeSession, map[string]struct{}, *jsonrpc.Error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	session := b.sessions[sessionID]
	if session == nil {
		session = &bridgeSession{seenResponseIDs: map[string]struct{}{}}
		b.sessions[sessionID] = session
	}
	if session.activeCancel != nil {
		return nil, nil, jsonrpc.InvalidRequest("a Microsoft Copilot turn is already in progress for this session", nil)
	}
	session.activeCancel = cancel
	session.cancelled = false
	seen := make(map[string]struct{}, len(session.seenResponseIDs))
	for id := range session.seenResponseIDs {
		seen[id] = struct{}{}
	}
	return session, seen, nil
}

func (b *acpBridge) endTurn(session *bridgeSession) {
	b.mu.Lock()
	session.activeCancel = nil
	b.mu.Unlock()
}

func (b *acpBridge) isTurnCancelled(session *bridgeSession) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return session.cancelled
}

func (b *acpBridge) recordSeenResponses(session *bridgeSession, ids []string) {
	if len(ids) == 0 {
		return
	}
	b.mu.Lock()
	for _, id := range ids {
		session.seenResponseIDs[id] = struct{}{}
	}
	b.mu.Unlock()
}

func (b *acpBridge) sendMessageChunk(ctx context.Context, sessionID acpschema.SessionID, messageID, text string) error {
	if text == "" {
		return nil
	}
	update := struct {
		SessionUpdate acpschema.SessionUpdateKind `json:"sessionUpdate"`
		Content       acpschema.TextContentBlock  `json:"content"`
		MessageID     string                      `json:"messageId,omitempty"`
	}{
		SessionUpdate: acpschema.SessionUpdateAgentMessageChunk,
		Content: acpschema.TextContentBlock{
			Kind:        acpschema.ContentBlockText,
			TextContent: acpschema.TextContent{Text: text},
		},
		MessageID: messageID,
	}
	return b.peer.Notify(ctx, acpschema.ClientMethodSessionUpdate, struct {
		SessionID acpschema.SessionID `json:"sessionId"`
		Update    any                 `json:"update"`
	}{
		SessionID: sessionID,
		Update:    update,
	})
}

func buildChatRequest(blocks []acpschema.ContentBlock) (ChatRequest, error) {
	var textParts []string
	var files []ContextFile
	for _, raw := range blocks {
		block, err := acpschema.DecodeContentBlock(raw)
		if err != nil {
			return ChatRequest{}, fmt.Errorf("decode prompt block: %w", err)
		}
		switch value := block.(type) {
		case acpschema.TextContentBlock:
			if text := strings.TrimSpace(value.Text); text != "" {
				textParts = append(textParts, value.Text)
			}
		case acpschema.ResourceLinkContentBlock:
			uri := strings.TrimSpace(value.URI)
			if isSharePointFileURI(uri) {
				files = append(files, ContextFile{URI: uri})
				continue
			}
			target := firstNonEmpty(strings.TrimSpace(value.Name), uri)
			return ChatRequest{}, fmt.Errorf("Microsoft Copilot Work/School only supports text prompts and Microsoft 365 SharePoint/OneDrive work URLs, not attachment %q", target)
		default:
			return ChatRequest{}, errors.New("Microsoft Copilot Work/School Chat API supports text prompts only, not image or audio attachments")
		}
	}
	promptText := strings.TrimSpace(strings.Join(textParts, "\n\n"))
	if promptText == "" {
		return ChatRequest{}, errors.New("text prompt is required")
	}
	req := ChatRequest{
		Message: RequestMessage{Text: promptText},
	}
	if len(files) > 0 {
		req.ContextualResources = &ContextualResources{Files: files}
	}
	return req, nil
}

func isSharePointFileURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "sharepoint.com" || strings.HasSuffix(host, ".sharepoint.com")
}

type streamedChunk struct {
	messageID string
	text      string
}

type streamTracker struct {
	priorResponseIDs map[string]struct{}
	emittedByKey     map[string]string
	turnResponseIDs  []string
	seenThisTurn     map[string]struct{}
	lastMessageID    string
	emittedAny       bool
	allEmitted       strings.Builder
}

func newStreamTracker(prior map[string]struct{}) *streamTracker {
	return &streamTracker{
		priorResponseIDs: prior,
		emittedByKey:     map[string]string{},
		seenThisTurn:     map[string]struct{}{},
	}
}

func (t *streamTracker) advance(conv Conversation) ([]streamedChunk, error) {
	candidates := currentTurnResponses(conv.Messages, t.priorResponseIDs)
	var chunks []streamedChunk
	for index, msg := range candidates {
		if msg.ID != "" {
			if _, ok := t.seenThisTurn[msg.ID]; !ok {
				t.seenThisTurn[msg.ID] = struct{}{}
				t.turnResponseIDs = append(t.turnResponseIDs, msg.ID)
			}
		}
		key := msg.ID
		if key == "" {
			key = fmt.Sprintf("idx:%d", index)
		}
		prev := t.emittedByKey[key]
		delta, err := cumulativeDelta(prev, msg.Text)
		if err != nil {
			if conv.DisengagedForRAI() {
				continue
			}
			return nil, err
		}
		if delta == "" {
			continue
		}
		if t.emittedAny && prev == "" {
			delta = "\n\n" + delta
		}
		t.emittedByKey[key] = msg.Text
		t.lastMessageID = msg.ID
		t.emittedAny = true
		t.allEmitted.WriteString(delta)
		chunks = append(chunks, streamedChunk{messageID: msg.ID, text: delta})
	}
	return chunks, nil
}

func currentTurnResponses(messages []ConversationMessage, prior map[string]struct{}) []ConversationMessage {
	start := 0
	for i, msg := range messages {
		if msg.IsRequest() {
			start = i + 1
		}
	}
	var out []ConversationMessage
	for _, msg := range messages[start:] {
		if !msg.IsResponse() {
			continue
		}
		if msg.ID != "" {
			if _, seen := prior[msg.ID]; seen {
				continue
			}
		}
		out = append(out, msg)
	}
	return out
}

func cumulativeDelta(prev, next string) (string, error) {
	if next == prev {
		return "", nil
	}
	if prev == "" {
		return next, nil
	}
	if strings.HasPrefix(next, prev) {
		return next[len(prev):], nil
	}
	return "", errors.New("Microsoft 365 Copilot revised an already-streamed response snapshot in place, which ACP append-only streaming cannot apply cleanly")
}

func collectAttributions(messages []ConversationMessage) []MessageAttribution {
	var out []MessageAttribution
	for _, msg := range messages {
		out = append(out, msg.Attributions...)
	}
	return out
}

var markdownLabelReplacer = strings.NewReplacer("\r", " ", "\n", " ", "[", "\\[", "]", "\\]")

func formatAttributions(attributions []MessageAttribution, existing string, emittedAny bool) string {
	seen := map[string]struct{}{}
	var lines []string
	for _, item := range attributions {
		link := strings.TrimSpace(item.SeeMoreWebURL)
		if link == "" || strings.ContainsAny(link, " \t\r\n<>") {
			continue
		}
		parsed, err := url.Parse(link)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		if strings.Contains(existing, link) {
			continue
		}
		if _, dup := seen[link]; dup {
			continue
		}
		seen[link] = struct{}{}
		label := strings.TrimSpace(markdownLabelReplacer.Replace(item.ProviderDisplayName))
		if label == "" {
			label = parsed.Host
		}
		lines = append(lines, fmt.Sprintf("- [%s](<%s>)", label, link))
	}
	if len(lines) == 0 {
		return ""
	}
	prefix := "\n\nSources:\n"
	if !emittedAny {
		prefix = "Sources:\n"
	}
	return prefix + strings.Join(lines, "\n")
}

func mapBridgeError(err error) *jsonrpc.Error {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return &jsonrpc.Error{
			Code:    int(acpschema.ErrorCode32000),
			Message: authErr.Error(),
		}
	}
	return jsonrpc.InternalError(err.Error(), nil)
}
