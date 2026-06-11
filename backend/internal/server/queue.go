package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type queueRequest struct {
	Op       string   `json:"op,omitempty"`
	Messages []string `json:"messages,omitempty"`
	Text     string   `json:"text,omitempty"`
	Expected string   `json:"expected,omitempty"`
	Index    int      `json:"index,omitempty"`
	From     int      `json:"from,omitempty"`
	To       int      `json:"to,omitempty"`
}

func (s *Server) handleQueueAction(w http.ResponseWriter, r *http.Request, session storage.Session) {
	var req queueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.op() == "steer" {
		updated, err := s.steerQueuedPrompt(session.ID, req)
		if err != nil {
			writeQueueError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, canonicalSessionResponse(updated))
		return
	}
	updated, err := s.mutateSessionQueue(session.ID, req)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	s.publishMessagesChanged(session.ID)
	writeJSON(w, http.StatusOK, canonicalSessionResponse(updated))
	if updated.Status == storage.StatusIdle && len(updated.QueuedMessages) > 0 && s.canStartQueuedPrompt(updated) {
		s.drainQueueSoon(updated.ID)
	}
}

func (s *Server) mutateSessionQueue(sessionID string, req queueRequest) (storage.Session, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, err
	}
	queue, err := applyQueueMutation(session.QueuedMessages, req)
	if err != nil {
		return storage.Session{}, err
	}
	session.QueuedMessages = queue
	if queueMutationTouchesAttention(req, queue) {
		storage.MarkSessionAttention(&session, time.Now().UTC())
	}
	if err := s.Store.SaveSession(session); err != nil {
		return storage.Session{}, err
	}
	return s.Store.LoadSession(sessionID)
}

func queueMutationTouchesAttention(req queueRequest, queue []string) bool {
	switch req.op() {
	case "", "replace":
		return len(queue) > 0
	case "append":
		return true
	default:
		return false
	}
}

func applyQueueMutation(queue []string, req queueRequest) ([]string, error) {
	queue = sanitizeQueuedMessages(queue)
	switch req.op() {
	case "", "replace":
		return sanitizeQueuedMessages(req.Messages), nil
	case "append":
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return queue, queueInputError{"queued prompt text is required"}
		}
		return append(queue, text), nil
	case "delete":
		if err := validateQueueIndex(queue, req.Index, req.Expected); err != nil {
			return queue, err
		}
		return append(append([]string(nil), queue[:req.Index]...), queue[req.Index+1:]...), nil
	case "edit":
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return queue, queueInputError{"queued prompt text is required"}
		}
		if err := validateQueueIndex(queue, req.Index, req.Expected); err != nil {
			return queue, err
		}
		next := append([]string(nil), queue...)
		next[req.Index] = text
		return next, nil
	case "move":
		if err := validateQueueIndex(queue, req.From, req.Expected); err != nil {
			return queue, err
		}
		if req.To < 0 || req.To >= len(queue) {
			return queue, queueInputError{"queued prompt target index out of range"}
		}
		if req.From == req.To {
			return queue, nil
		}
		next := append([]string(nil), queue...)
		item := next[req.From]
		next = append(next[:req.From], next[req.From+1:]...)
		next = append(next[:req.To], append([]string{item}, next[req.To:]...)...)
		return next, nil
	default:
		return queue, queueInputError{fmt.Sprintf("unknown queue operation %q", req.Op)}
	}
}

func (req queueRequest) op() string {
	return strings.TrimSpace(req.Op)
}

func validateQueueIndex(queue []string, index int, expected string) error {
	if index < 0 || index >= len(queue) {
		return queueInputError{"queued prompt index out of range"}
	}
	if expected = strings.TrimSpace(expected); expected != "" && queue[index] != expected {
		return queueInputError{"queued prompt changed; refresh and try again"}
	}
	return nil
}

type queueInputError struct {
	message string
}

func (e queueInputError) Error() string {
	return e.message
}

func writeQueueError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var inputErr queueInputError
	if errors.As(err, &inputErr) {
		status = http.StatusBadRequest
	}
	writeError(w, status, err)
}

func (s *Server) HandleACPTurnFinished(_ context.Context, job acp.Job) {
	if job.ID == "" {
		return
	}
	shouldDrain := job.State == acp.StateIdle
	if status := storage.SessionStatusForACPState(job.State); status != "" {
		s.setSessionStatusWithError(storage.Session{ID: job.ID}, status, job.Error)
	}
	s.publishMessagesChanged(job.ID)
	if shouldDrain {
		s.drainQueueSoon(job.ID)
	}
}

func (s *Server) drainQueueSoon(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	go s.drainQueuedPrompt(context.Background(), sessionID)
}

func (s *Server) drainQueuedPrompt(ctx context.Context, sessionID string) {
	session, prompt, ok, err := s.claimQueuedPrompt(sessionID)
	if err != nil {
		s.logger().Error("queued prompt claim failed", "session", sessionID, "error", err)
		return
	}
	if !ok {
		return
	}
	s.publishMessagesChanged(session.ID)
	if err := s.startQueuedPrompt(ctx, session, prompt); err != nil {
		s.logger().Error("queued prompt start failed", "session", session.ID, "error", err)
		s.restoreQueuedPrompt(session.ID, prompt, 0, err.Error())
	}
}

func (s *Server) claimQueuedPrompt(sessionID string) (storage.Session, string, bool, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, "", false, err
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if s.sessionRuntimeRunning(session) || session.Status != storage.StatusIdle {
		return session, "", false, nil
	}
	prompts := sanitizeQueuedMessages(session.QueuedMessages)
	if len(prompts) == 0 {
		if len(session.QueuedMessages) > 0 {
			session.QueuedMessages = nil
			if err := s.Store.SaveSession(session); err != nil {
				return storage.Session{}, "", false, err
			}
		}
		return session, "", false, nil
	}

	prompt := prompts[0]
	session.QueuedMessages = prompts[1:]
	session.Status = storage.StatusRunning
	session.Error = ""
	storage.MarkSessionAttention(&session, time.Now().UTC())
	if session.Title == "" {
		session.Title = titleFromMessage(prompt)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return storage.Session{}, "", false, err
	}
	return session, prompt, true, nil
}

type steeredQueuedPrompt struct {
	session    storage.Session
	prompt     string
	index      int
	interrupts bool
}

func (s *Server) steerQueuedPrompt(sessionID string, req queueRequest) (storage.Session, error) {
	claimed, err := s.claimSteeredQueuedPrompt(sessionID, req)
	if err != nil {
		return storage.Session{}, err
	}
	s.publishMessagesChanged(claimed.session.ID)
	go s.dispatchSteeredQueuedPrompt(claimed)
	return s.Store.LoadSession(sessionID)
}

func (s *Server) dispatchSteeredQueuedPrompt(claimed steeredQueuedPrompt) {
	ctx := context.Background()
	cancel := func() {}
	if claimed.interrupts || claimed.session.Runtime == storage.RuntimeACP {
		ctx, cancel = serverActionContext()
	}
	defer cancel()
	if err := s.startSteeredQueuedPrompt(ctx, claimed); err != nil {
		s.logger().Error("queued prompt steer failed", "session", claimed.session.ID, "error", err)
		s.restoreQueuedPrompt(claimed.session.ID, claimed.prompt, claimed.index, err.Error())
	}
}

func (s *Server) claimSteeredQueuedPrompt(sessionID string, req queueRequest) (steeredQueuedPrompt, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return steeredQueuedPrompt{}, err
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	queue := sanitizeQueuedMessages(session.QueuedMessages)
	if err := validateQueueIndex(queue, req.Index, req.Expected); err != nil {
		return steeredQueuedPrompt{}, err
	}

	running := s.sessionRuntimeRunning(session)
	if running && session.Runtime != storage.RuntimeACP {
		return steeredQueuedPrompt{}, queueInputError{"queued prompts can only steer running ACP sessions"}
	}
	if !running && !s.canStartQueuedPrompt(session) {
		return steeredQueuedPrompt{}, queueInputError{"session runtime is not configured"}
	}

	prompt := queue[req.Index]
	session.QueuedMessages = removeQueuedPrompt(queue, req.Index)
	session.Error = ""
	storage.MarkSessionAttention(&session, time.Now().UTC())
	if !running {
		session.Status = storage.StatusRunning
		if session.Title == "" {
			session.Title = titleFromMessage(prompt)
		}
	}
	if err := s.Store.SaveSession(session); err != nil {
		return steeredQueuedPrompt{}, err
	}
	return steeredQueuedPrompt{
		session:    session,
		prompt:     prompt,
		index:      req.Index,
		interrupts: running,
	}, nil
}

func (s *Server) startSteeredQueuedPrompt(ctx context.Context, claimed steeredQueuedPrompt) error {
	if claimed.interrupts {
		if s.ACP == nil {
			return fmt.Errorf("acp manager is not configured")
		}
		return s.ACP.AnswerInteractive(ctx, acp.InteractiveAnswer{
			Session: claimed.session.ID,
			Text:    claimed.prompt,
		})
	}
	return s.startQueuedPrompt(ctx, claimed.session, claimed.prompt)
}

func (s *Server) sessionRuntimeRunning(session storage.Session) bool {
	if session.Runtime == storage.RuntimeACP && s.ACP != nil {
		if job, err := s.ACP.Status(session.ID); err == nil {
			return job.State == acp.StateRunning || job.State == acp.StateStarting
		}
	}
	return session.Status == storage.StatusRunning
}

func (s *Server) canStartQueuedPrompt(session storage.Session) bool {
	switch session.Runtime {
	case "", storage.RuntimeNative:
		return s.Agent != nil
	case storage.RuntimeACP:
		return s.ACP != nil
	default:
		return false
	}
}

func (s *Server) startQueuedPrompt(ctx context.Context, session storage.Session, prompt string) error {
	switch session.Runtime {
	case "", storage.RuntimeNative:
		if s.Agent == nil {
			return fmt.Errorf("agent is not configured")
		}
		if s.runClaimedNativeSession(ctx, session, prompt) == storage.StatusIdle {
			s.drainQueueSoon(session.ID)
		}
		return nil
	case storage.RuntimeACP:
		if s.ACP == nil {
			return fmt.Errorf("acp manager is not configured")
		}
		if _, err := s.ACP.Send(ctx, acp.SendRequest{
			Session:     session.ID,
			Message:     prompt,
			Completion:  acp.CompletionAsync,
			Interactive: true,
		}); err != nil {
			return acpSendError(session, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown session runtime %q", session.Runtime)
	}
}

func (s *Server) restoreQueuedPrompt(sessionID, prompt string, index int, message string) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return
	}
	queue := sanitizeQueuedMessages(session.QueuedMessages)
	if index < 0 || index > len(queue) {
		index = len(queue)
	}
	queue = append(queue[:index], append([]string{prompt}, queue[index:]...)...)
	session.QueuedMessages = queue
	session.Status = storage.StatusError
	session.Error = firstNonEmpty(message, session.Error, "Queued prompt failed.")
	storage.MarkSessionAttention(&session, time.Now().UTC())
	_ = s.Store.SaveSession(session)
	s.publishMessagesChanged(sessionID)
}

func removeQueuedPrompt(prompts []string, index int) []string {
	return append(append([]string(nil), prompts[:index]...), prompts[index+1:]...)
}

func sanitizeQueuedMessages(messages []string) []string {
	if len(messages) == 0 {
		return nil
	}
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		out = append(out, message)
	}
	return out
}
