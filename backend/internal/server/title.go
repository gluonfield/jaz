package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const titleGenerationTimeout = 5 * time.Second

var sessionTitleOutput = &provider.StructuredOutput{
	Name:        "session_title",
	Description: "A concise display title for a chat thread.",
	Strict:      true,
	Schema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "A concise 2-6 word title without markdown or surrounding quotes.",
			},
		},
		"required": []string{"title"},
	},
}

type sessionTitleResponse struct {
	Title string `json:"title"`
}

func shouldGenerateTitleFromMessage(currentTitle, message string, existing []provider.Message) bool {
	if hasConversationMessages(existing) {
		return false
	}
	currentTitle = strings.TrimSpace(currentTitle)
	if currentTitle == "" {
		return true
	}
	message = strings.TrimSpace(message)
	return currentTitle == message || currentTitle == titleFromMessage(message)
}

func hasConversationMessages(messages []provider.Message) bool {
	for _, message := range messages {
		switch provider.MessageRole(message) {
		case "user", "assistant", "tool", "function":
			return true
		}
	}
	return false
}

func (s *Server) generateAndSaveSessionTitle(ctx context.Context, session storage.Session, message string) storage.Session {
	title, err := s.generateSessionTitle(ctx, session, message)
	if err != nil {
		s.logger().Debug("session title generation failed", "session", session.ID, "error", err)
		return session
	}

	unlock := s.lockSession(session.ID)
	defer unlock()
	current, err := s.Store.LoadSession(session.ID)
	if err != nil {
		s.logger().Debug("loading session after title generation failed", "session", session.ID, "error", err)
		return session
	}
	if current.Title != session.Title {
		return current
	}
	current.Title = title
	if err := s.Store.SaveSession(current); err != nil {
		s.logger().Debug("saving generated session title failed", "session", session.ID, "error", err)
		return session
	}
	s.publishMessagesChanged(current.ID)
	return current
}

func (s *Server) generateSessionTitle(ctx context.Context, session storage.Session, message string) (string, error) {
	if s.Agent == nil || s.Agent.Provider == nil {
		return "", fmt.Errorf("model provider is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, titleGenerationTimeout)
	defer cancel()

	resp, err := s.Agent.Provider.Complete(ctx, provider.Request{
		Provider:         session.ModelProvider,
		Model:            session.Model,
		Messages:         titlePrompt(message),
		StructuredOutput: sessionTitleOutput,
	})
	if err != nil {
		return "", err
	}
	var parsed sessionTitleResponse
	if err := json.Unmarshal([]byte(provider.MessageContent(resp.Message)), &parsed); err != nil {
		return "", err
	}
	title := cleanGeneratedTitle(parsed.Title)
	if title == "" {
		return "", fmt.Errorf("empty generated title")
	}
	return title, nil
}

func titlePrompt(message string) []provider.Message {
	return []provider.Message{
		provider.SystemMessage("Write concise thread titles for a coding assistant. Use 2-6 words. Preserve important proper nouns. Do not use markdown, surrounding quotes, or trailing punctuation."),
		provider.UserMessage("First user message:\n\n" + strings.TrimSpace(message)),
	}
}

func cleanGeneratedTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	title = strings.Trim(title, " \t\r\n\"'`")
	title = strings.TrimRight(title, ".,!?;:")
	runes := []rune(title)
	if len(runes) > 64 {
		title = strings.TrimSpace(string(runes[:64]))
		title = strings.TrimRight(title, ".,!?;:")
	}
	return title
}
