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
	Op      string                `json:"op,omitempty"`
	ID      string                `json:"id,omitempty"`
	IDs     []string              `json:"ids,omitempty"`
	Message storage.QueuedMessage `json:"message,omitempty"`
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
	if err := s.validateQueueAttachmentMutation(session.ID, req); err != nil {
		return storage.Session{}, err
	}
	session.QueuedMessages = s.assignQueuedMessageIDs(queue)
	if queueMutationTouchesAttention(req) {
		storage.MarkSessionAttention(&session, time.Now().UTC())
	}
	if err := s.Store.SaveSession(session); err != nil {
		return storage.Session{}, err
	}
	return s.Store.LoadSession(sessionID)
}

func queueMutationTouchesAttention(req queueRequest) bool {
	switch req.op() {
	case "append":
		return true
	default:
		return false
	}
}

func applyQueueMutation(queue []storage.QueuedMessage, req queueRequest) ([]storage.QueuedMessage, error) {
	queue = storage.CanonicalQueuedMessages(queue)
	switch req.op() {
	case "append":
		message := req.Message
		message.ID = ""
		msgs := storage.NormalizeQueuedMessages([]storage.QueuedMessage{message})
		if len(msgs) == 0 {
			return queue, queueInputError{"queued prompt text is required"}
		}
		return append(queue, msgs[0]), nil
	case "delete":
		index, err := queuedPromptIndex(queue, req.ID)
		if err != nil {
			return queue, err
		}
		return removeQueuedPrompt(queue, index), nil
	case "edit":
		text := strings.TrimSpace(req.Message.Text)
		if text == "" {
			return queue, queueInputError{"queued prompt text is required"}
		}
		index, err := queuedPromptIndex(queue, req.ID)
		if err != nil {
			return queue, err
		}
		next := append([]storage.QueuedMessage(nil), queue...)
		next[index].Text = text
		return next, nil
	case "reorder":
		return reorderQueuedPrompts(queue, req.IDs)
	default:
		return queue, queueInputError{fmt.Sprintf("unknown queue operation %q", req.Op)}
	}
}

func (req queueRequest) op() string {
	return strings.TrimSpace(req.Op)
}

func queuedPromptIndex(queue []storage.QueuedMessage, id string) (int, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1, queueInputError{"queued prompt id is required"}
	}
	for i, prompt := range queue {
		if prompt.ID == id {
			return i, nil
		}
	}
	return -1, queueInputError{"queued prompt changed; refresh and try again"}
}

func reorderQueuedPrompts(queue []storage.QueuedMessage, ids []string) ([]storage.QueuedMessage, error) {
	if len(ids) != len(queue) {
		return queue, queueInputError{"queued prompt order changed; refresh and try again"}
	}
	byID := make(map[string]storage.QueuedMessage, len(queue))
	for _, prompt := range queue {
		byID[prompt.ID] = prompt
	}
	next := make([]storage.QueuedMessage, 0, len(queue))
	seen := make(map[string]bool, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		prompt, ok := byID[id]
		if id == "" || !ok || seen[id] {
			return queue, queueInputError{"queued prompt order changed; refresh and try again"}
		}
		seen[id] = true
		next = append(next, prompt)
	}
	return next, nil
}

func (s *Server) validateQueueAttachmentMutation(sessionID string, req queueRequest) error {
	switch req.op() {
	case "append":
		if len(req.Message.AttachmentIDs) == 0 {
			return nil
		}
		_, err := s.resolveAttachments(sessionID, req.Message.AttachmentIDs)
		return err
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
	// The turn's token usage was persisted during the turn; tell open pages to
	// refetch the session so the usage meter reflects it without a reload.
	s.publishSessionChanged(job.ID)
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
		return
	}
	s.publishMessagesChanged(session.ID)
}

func (s *Server) claimQueuedPrompt(sessionID string) (storage.Session, storage.QueuedMessage, bool, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, storage.QueuedMessage{}, false, err
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeACP
	}
	if s.sessionRuntimeRunning(session) || session.Status != storage.StatusIdle {
		return session, storage.QueuedMessage{}, false, nil
	}
	prompts := s.assignQueuedMessageIDs(storage.CanonicalQueuedMessages(session.QueuedMessages))
	if len(prompts) == 0 {
		if len(session.QueuedMessages) > 0 {
			session.QueuedMessages = nil
			if err := s.Store.SaveSession(session); err != nil {
				return storage.Session{}, storage.QueuedMessage{}, false, err
			}
		}
		return session, storage.QueuedMessage{}, false, nil
	}

	prompt := prompts[0]
	session.QueuedMessages = s.assignQueuedMessageIDs(prompts[1:])
	session.Status = storage.StatusRunning
	session.Error = ""
	storage.MarkSessionAttention(&session, time.Now().UTC())
	if session.Title == "" {
		session.Title = titleFromMessage(prompt.Text)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return storage.Session{}, storage.QueuedMessage{}, false, err
	}
	s.maybeGenerateSessionTitle(session, prompt.Text)
	return session, prompt, true, nil
}

type steeredQueuedPrompt struct {
	session    storage.Session
	prompt     storage.QueuedMessage
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
	if err := s.startSteeredQueuedPrompt(claimed); err != nil {
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
		session.Runtime = storage.RuntimeACP
	}
	queue := s.assignQueuedMessageIDs(storage.CanonicalQueuedMessages(session.QueuedMessages))
	index, err := queuedPromptIndex(queue, req.ID)
	if err != nil {
		return steeredQueuedPrompt{}, err
	}

	running := s.sessionRuntimeRunning(session)
	if running && session.Runtime != storage.RuntimeACP {
		return steeredQueuedPrompt{}, queueInputError{"queued prompts can only steer running ACP sessions"}
	}
	if !running && !s.canStartQueuedPrompt(session) {
		return steeredQueuedPrompt{}, queueInputError{"session runtime is not configured"}
	}

	prompt := queue[index]
	session.QueuedMessages = s.assignQueuedMessageIDs(removeQueuedPrompt(queue, index))
	pending := prompt
	session.PendingSteer = &pending
	session.Error = ""
	storage.MarkSessionAttention(&session, time.Now().UTC())
	if !running {
		session.Status = storage.StatusRunning
		if session.Title == "" {
			session.Title = titleFromMessage(prompt.Text)
		}
	}
	if err := s.Store.SaveSession(session); err != nil {
		return steeredQueuedPrompt{}, err
	}
	if !running {
		s.maybeGenerateSessionTitle(session, prompt.Text)
	}
	return steeredQueuedPrompt{
		session:    session,
		prompt:     prompt,
		index:      index,
		interrupts: running,
	}, nil
}

func (s *Server) startSteeredQueuedPrompt(claimed steeredQueuedPrompt) error {
	if claimed.interrupts {
		if s.ACP == nil {
			return fmt.Errorf("acp manager is not configured")
		}
		ctx, cancel := serverActionContext()
		_, err := s.ACP.Cancel(ctx, claimed.session.ID)
		cancel()
		if err != nil {
			return err
		}
		s.setSessionStatus(storage.Session{ID: claimed.session.ID}, storage.StatusRunning)
		s.publishSessionChanged(claimed.session.ID)
	}
	ctx, cancel := serverActionContext()
	defer cancel()
	if err := s.startQueuedPrompt(ctx, claimed.session, claimed.prompt); err != nil {
		return err
	}
	if err := s.clearPendingSteerMessage(claimed.session.ID); err != nil {
		s.logger().Error("pending steer clear failed", "session", claimed.session.ID, "error", err)
	}
	s.publishMessagesChanged(claimed.session.ID)
	return nil
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
	case "", storage.RuntimeACP:
		return s.ACP != nil
	default:
		return false
	}
}

func (s *Server) startQueuedPrompt(ctx context.Context, session storage.Session, prompt storage.QueuedMessage) error {
	attachments, err := s.resolveAttachments(session.ID, prompt.AttachmentIDs)
	if err != nil {
		return err
	}
	switch session.Runtime {
	case "", storage.RuntimeACP:
		if s.ACP == nil {
			return fmt.Errorf("acp manager is not configured")
		}
		if err := s.ensureManagedWorktree(ctx, session); err != nil {
			return err
		}
		if _, err := s.ACP.Send(ctx, acp.SendRequest{
			Session:       session.ID,
			Message:       prompt.Text,
			Contexts:      prompt.Contexts,
			Attachments:   attachments,
			Completion:    acp.CompletionAsync,
			Interactive:   true,
			PlanRequested: prompt.PlanRequested,
		}); err != nil {
			return acpSendError(session, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown session runtime %q", session.Runtime)
	}
}

func (s *Server) restoreQueuedPrompt(sessionID string, prompt storage.QueuedMessage, index int, message string) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return
	}
	queue := s.assignQueuedMessageIDs(storage.CanonicalQueuedMessages(session.QueuedMessages))
	if index < 0 || index > len(queue) {
		index = len(queue)
	}
	queue = append(queue[:index], append([]storage.QueuedMessage{prompt}, queue[index:]...)...)
	session.QueuedMessages = s.assignQueuedMessageIDs(queue)
	session.PendingSteer = nil
	session.Status = storage.StatusError
	session.Error = firstNonEmpty(message, session.Error, "Queued prompt failed.")
	storage.MarkSessionAttention(&session, time.Now().UTC())
	_ = s.Store.SaveSession(session)
	s.publishMessagesChanged(sessionID)
}

func (s *Server) clearPendingSteerMessage(sessionID string) error {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return err
	}
	if session.PendingSteer == nil {
		return nil
	}
	session.PendingSteer = nil
	return s.Store.SaveSession(session)
}

func removeQueuedPrompt(prompts []storage.QueuedMessage, index int) []storage.QueuedMessage {
	return append(append([]storage.QueuedMessage(nil), prompts[:index]...), prompts[index+1:]...)
}

func (s *Server) assignQueuedMessageIDs(queue []storage.QueuedMessage) []storage.QueuedMessage {
	if len(queue) == 0 {
		return nil
	}
	next := append([]storage.QueuedMessage(nil), queue...)
	seen := make(map[string]bool, len(next))
	for i := range next {
		id := strings.TrimSpace(next[i].ID)
		if id == "" || seen[id] {
			id = "queue-" + s.Store.NewSessionID()
		}
		next[i].ID = id
		seen[id] = true
	}
	return next
}
