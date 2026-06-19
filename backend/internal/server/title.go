package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

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

func (s *Server) maybeGenerateSessionTitle(session storage.Session, message string) {
	existing, err := s.Store.LoadMessages(session.ID)
	if err != nil {
		s.logger().Debug("loading messages before title generation failed", "session", session.ID, "error", err)
		return
	}
	if !shouldGenerateTitleFromMessage(session.Title, message, existing) {
		return
	}
	go func() {
		ctx, cancel := serverActionContext()
		defer cancel()
		s.generateAndSaveSessionTitle(ctx, session, message)
	}()
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
	s.publishSessionChanged(current.ID)
	return current
}

func (s *Server) generateSessionTitle(ctx context.Context, session storage.Session, message string) (string, error) {
	text, err := s.ACP.RunUtilityPrompt(ctx, acp.UtilityPromptRequest{
		ACPAgent:        sessionACPAgent(session),
		Directory:       sessionDirectory(session),
		Message:         titlePrompt(message),
		ModelProvider:   session.ModelProvider,
		Model:           session.Model,
		ReasoningEffort: "none",
	})
	if err != nil {
		return "", err
	}
	title := cleanGeneratedTitle(parseGeneratedTitle(text))
	if title == "" {
		return "", fmt.Errorf("empty generated title")
	}
	return title, nil
}

func sessionACPAgent(session storage.Session) string {
	if session.RuntimeRef != nil {
		if agent := acp.CanonicalAgentName(session.RuntimeRef.Agent); agent != "" {
			return agent
		}
	}
	return acp.CanonicalAgentName(session.ModelProvider)
}

func sessionDirectory(session storage.Session) string {
	if session.RuntimeRef == nil {
		return "."
	}
	return firstNonEmpty(session.RuntimeRef.Cwd, session.RuntimeRef.ProjectPath, ".")
}

func titlePrompt(message string) string {
	return "Write a concise 2-6 word display title for this coding-assistant thread. Preserve important proper nouns. Return exactly JSON in this shape and no other text: {\"title\":\"...\"}\n\nFirst user message:\n\n" + strings.TrimSpace(message)
}

func parseGeneratedTitle(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	var parsed sessionTitleResponse
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		return parsed.Title
	}
	if before, after, ok := strings.Cut(text, "```"); ok {
		if strings.TrimSpace(before) == "" {
			return parseGeneratedTitle(after)
		}
	}
	return strings.TrimPrefix(text, "Title:")
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
