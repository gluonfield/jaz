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
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionview"
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
		writeJSON(w, http.StatusOK, sessionview.Public(updated))
		return
	}
	updated, err := s.updateSessionQueue(session.ID, req)
	if err != nil {
		writeQueueError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionview.Public(updated))
}

func (s *Server) updateSessionQueue(sessionID string, req queueRequest) (storage.Session, error) {
	updated, err := s.mutateSessionQueue(sessionID, req)
	if err != nil {
		return storage.Session{}, err
	}
	s.publishMessagesChanged(sessionID)
	if updated.Status == storage.StatusIdle && len(updated.QueuedMessages) > 0 && s.canStartQueuedTurn(updated) {
		s.drainQueueSoon(sessionID)
	}
	return updated, nil
}

func (s *Server) mutateSessionQueue(sessionID string, req queueRequest) (storage.Session, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, err
	}
	internalQueue, publicQueue := splitQueuedMessages(session.QueuedMessages)
	queue, err := applyQueueMutation(publicQueue, req)
	if err != nil {
		return storage.Session{}, err
	}
	if req.op() == "append" && len(queue) > 0 {
		if err := s.validateQueuedMessage(session, queue[len(queue)-1]); err != nil {
			return storage.Session{}, err
		}
	}
	if err := s.validateQueueAttachmentMutation(session.ID, req); err != nil {
		return storage.Session{}, err
	}
	session.QueuedMessages = s.assignQueuedMessageIDs(append(internalQueue, queue...))
	if queueMutationTouchesAttention(req) {
		storage.MarkSessionAttention(&session, time.Now().UTC())
	}
	if err := s.Store.SaveSession(session); err != nil {
		return storage.Session{}, err
	}
	return s.Store.LoadSession(sessionID)
}

func splitQueuedMessages(queue []storage.QueuedMessage) ([]storage.QueuedMessage, []storage.QueuedMessage) {
	queue = storage.CanonicalQueuedMessages(queue)
	internalQueue := make([]storage.QueuedMessage, 0, len(queue))
	publicQueue := make([]storage.QueuedMessage, 0, len(queue))
	for _, message := range queue {
		if message.IsInternal() {
			internalQueue = append(internalQueue, message)
		} else {
			publicQueue = append(publicQueue, message)
		}
	}
	return internalQueue, publicQueue
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
		if message.Action != "" && !storage.ValidQueuedAction(message.Action) {
			return queue, queueInputError{"unknown queued action"}
		}
		message.ID = ""
		message = message.AsPublic()
		msgs := storage.NormalizeQueuedMessages([]storage.QueuedMessage{message})
		if len(msgs) == 0 {
			return queue, queueInputError{"queued prompt content is required"}
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
		next[index] = next[index].AsPublic()
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
	unlock := s.lockSession(job.ID)
	turnCompleted := job.State == acp.StateIdle
	var completionErr error
	if turnCompleted {
		completionErr = s.Store.CompleteSession(job.ID, time.Now().UTC())
	} else if status := storage.SessionStatusForACPState(job.State); status != "" {
		s.setSessionStatusWithError(storage.Session{ID: job.ID}, status, job.Error)
	}
	unlock()
	if completionErr != nil {
		s.logger().Error("session completion update failed", "session", job.ID, "error", completionErr)
		return
	}
	s.publishMessagesChanged(job.ID)
	// The turn's token usage was persisted during the turn; tell open pages to
	// refetch the session so the usage meter reflects it without a reload.
	s.publishSessionChanged(job.ID)
	if turnCompleted {
		s.drainQueueSoon(job.ID)
	}
}

func (s *Server) drainQueueSoon(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	session, prompt, ok, err := s.claimNextTurn(sessionID)
	if err != nil {
		s.logger().Error("next turn claim failed", "session", sessionID, "error", err)
		return
	}
	if ok {
		go s.runClaimedTurn(context.Background(), session, prompt)
	}
}

func (s *Server) StartInternalTurn(_ context.Context, sessionID, message string) error {
	prompt, ok := storage.NormalizeQueuedMessage(storage.NewInternalQueuedMessage(message))
	if !ok {
		return fmt.Errorf("message is required")
	}

	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return err
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeACP
	}
	if !s.canStartQueuedPrompt(session) {
		return fmt.Errorf("session runtime is not configured")
	}
	session.QueuedMessages = s.assignQueuedMessageIDs(insertInternalQueuedPrompt(session.QueuedMessages, prompt))
	idle := session.Status == storage.StatusIdle && !s.sessionRuntimeRunning(session)
	if err := s.Store.SaveSession(session); err != nil {
		return err
	}
	if idle {
		s.drainQueueSoon(session.ID)
	}
	return nil
}

func (s *Server) drainQueuedPrompt(ctx context.Context, sessionID string) {
	session, prompt, ok, err := s.claimNextTurn(sessionID)
	if err != nil {
		s.logger().Error("next turn claim failed", "session", sessionID, "error", err)
		return
	}
	if !ok {
		return
	}
	s.runClaimedTurn(ctx, session, prompt)
}

func (s *Server) runClaimedTurn(ctx context.Context, session storage.Session, prompt *storage.QueuedMessage) {
	if prompt == nil {
		s.startGoalContinuation(ctx, session)
		return
	}
	s.publishMessagesChanged(session.ID)
	if prompt.IsAction() {
		async, err := s.startQueuedAction(ctx, session, *prompt)
		if err != nil {
			s.logger().Error("queued action failed", "session", session.ID, "action", prompt.Action, "error", err)
			s.restoreQueuedPrompt(session.ID, *prompt, 0, err.Error())
			return
		}
		if !async {
			s.finishQueuedAction(session.ID, prompt.Action)
		}
		return
	}
	if err := s.startQueuedPrompt(ctx, session, *prompt); err != nil {
		s.logger().Error("queued prompt start failed", "session", session.ID, "error", err)
		s.restoreQueuedPrompt(session.ID, *prompt, 0, err.Error())
		return
	}
	s.publishMessagesChanged(session.ID)
}

func (s *Server) startGoalContinuation(ctx context.Context, session storage.Session) {
	s.publishSessionChanged(session.ID)
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		s.failGoalContinuation(session.ID, err)
		return
	}
	if _, err := s.ACP.ContinueGoal(ctx, session.ID); err != nil {
		s.failGoalContinuation(session.ID, err)
	}
}

func (s *Server) failGoalContinuation(sessionID string, err error) {
	s.logger().Error("goal continuation start failed", "session", sessionID, "error", err)
	s.setSessionStatus(storage.Session{ID: sessionID}, storage.StatusIdle)
	s.publishSessionChanged(sessionID)
}

// claimNextTurn atomically selects the next turn for an idle session: the oldest
// queued prompt, or a goal continuation when the queue is empty but an active
// goal remains. A nil prompt with ok=true means goal continuation.
func (s *Server) claimNextTurn(sessionID string) (storage.Session, *storage.QueuedMessage, bool, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, nil, false, err
	}
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeACP
	}
	if s.sessionRuntimeRunning(session) || session.Status != storage.StatusIdle {
		return session, nil, false, nil
	}
	prompts := s.assignQueuedMessageIDs(storage.CanonicalQueuedMessages(session.QueuedMessages))
	if len(prompts) == 0 {
		// Empty queue still yields work when a goal is live; otherwise drop any
		// stale (non-canonical) entries and report nothing to run.
		if goal.Continuable(session.Goal) && session.Runtime == storage.RuntimeACP && s.ACP != nil {
			session.QueuedMessages = nil
			markSessionRunning(&session)
			if err := s.Store.SaveSession(session); err != nil {
				return storage.Session{}, nil, false, err
			}
			return session, nil, true, nil
		}
		if len(session.QueuedMessages) > 0 {
			session.QueuedMessages = nil
			if err := s.Store.SaveSession(session); err != nil {
				return storage.Session{}, nil, false, err
			}
		}
		return session, nil, false, nil
	}

	index := nextQueuedPromptIndex(prompts)
	prompt := prompts[index]
	if err := s.validateQueuedMessage(session, prompt); err != nil {
		return storage.Session{}, nil, false, err
	}
	session.QueuedMessages = s.assignQueuedMessageIDs(removeQueuedPrompt(prompts, index))
	markSessionRunning(&session)
	if session.Title == "" && !prompt.IsInternal() && !prompt.IsAction() {
		session.Title = titleFromMessage(prompt.Text)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return storage.Session{}, nil, false, err
	}
	if !prompt.IsInternal() && !prompt.IsAction() {
		s.maybeGenerateSessionTitle(session, prompt.Text)
	}
	return session, &prompt, true, nil
}

func insertInternalQueuedPrompt(queue []storage.QueuedMessage, prompt storage.QueuedMessage) []storage.QueuedMessage {
	prompt = prompt.AsInternal()
	queue = storage.CanonicalQueuedMessages(queue)
	index := 0
	for index < len(queue) && queue[index].IsInternal() {
		index++
	}
	next := append([]storage.QueuedMessage(nil), queue...)
	next = append(next, storage.QueuedMessage{})
	copy(next[index+1:], next[index:])
	next[index] = prompt
	return storage.CanonicalQueuedMessages(next)
}

func nextQueuedPromptIndex(prompts []storage.QueuedMessage) int {
	for i, prompt := range prompts {
		if prompt.IsInternal() {
			return i
		}
	}
	return 0
}

func markSessionRunning(session *storage.Session) {
	session.Status = storage.StatusRunning
	session.Error = ""
	storage.MarkSessionAttention(session, time.Now().UTC())
}

type steeredQueuedPrompt struct {
	session     storage.Session
	prompt      storage.QueuedMessage
	publicIndex int
	interrupts  bool
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
		s.restoreQueuedPrompt(claimed.session.ID, claimed.prompt, claimed.publicIndex, err.Error())
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
	internalQueue, publicQueue := splitQueuedMessages(queue)
	index, err := queuedPromptIndex(publicQueue, req.ID)
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

	prompt := publicQueue[index]
	if prompt.IsAction() {
		return steeredQueuedPrompt{}, queueInputError{"queued actions cannot be steered"}
	}
	if err := s.validateQueuedPrompt(session, prompt); err != nil {
		return steeredQueuedPrompt{}, err
	}
	session.QueuedMessages = s.assignQueuedMessageIDs(append(internalQueue, removeQueuedPrompt(publicQueue, index)...))
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
		session:     session,
		prompt:      prompt,
		publicIndex: index,
		interrupts:  running,
	}, nil
}

func (s *Server) startSteeredQueuedPrompt(claimed steeredQueuedPrompt) error {
	if claimed.interrupts {
		if s.ACP == nil {
			return fmt.Errorf("acp manager is not configured")
		}
		ctx, cancel := serverActionContext()
		err := s.steerRunningQueuedPrompt(ctx, claimed.session, claimed.prompt)
		cancel()
		if err == nil {
			if err := s.clearPendingSteerMessage(claimed.session.ID); err != nil {
				s.logger().Error("pending steer clear failed", "session", claimed.session.ID, "error", err)
			}
			s.publishMessagesChanged(claimed.session.ID)
			return nil
		}
		if !errors.Is(err, acp.ErrPromptQueueingUnsupported) {
			return err
		}
		ctx, cancel = serverActionContext()
		_, err = s.ACP.Cancel(ctx, claimed.session.ID)
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

func (s *Server) steerRunningQueuedPrompt(ctx context.Context, session storage.Session, prompt storage.QueuedMessage) error {
	if err := s.validateGoalRequest(session, prompt.GoalRequested); err != nil {
		return err
	}
	attachments, err := s.resolveAttachments(session.ID, prompt.AttachmentIDs)
	if err != nil {
		return err
	}
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		return err
	}
	_, err = s.ACP.Steer(ctx, acp.SteerRequest{
		Session:       session.ID,
		Message:       prompt.Text,
		Contexts:      prompt.Contexts,
		Attachments:   attachments,
		GoalRequested: prompt.GoalRequested,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, acp.ErrPromptQueueingUnsupported) {
		return err
	}
	return acpSendError(session, err)
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

func (s *Server) canStartQueuedTurn(session storage.Session) bool {
	prompts := storage.CanonicalQueuedMessages(session.QueuedMessages)
	if len(prompts) == 0 {
		return false
	}
	prompt := prompts[nextQueuedPromptIndex(prompts)]
	if prompt.IsAction() {
		return s.canStartQueuedAction(session, prompt.Action)
	}
	return s.canStartQueuedPrompt(session)
}

func (s *Server) startQueuedPrompt(ctx context.Context, session storage.Session, prompt storage.QueuedMessage) error {
	if prompt.IsAction() {
		return fmt.Errorf("queued action reached prompt runner")
	}
	if err := s.validateGoalRequest(session, prompt.GoalRequested); err != nil {
		return err
	}
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
		if prompt.IsInternal() {
			if _, err := s.ACP.StartInternalTurn(ctx, acp.InternalTurnRequest{
				Session: session.ID,
				Message: prompt.Text,
			}); err != nil {
				return acpSendError(session, err)
			}
			return nil
		}
		if _, err := s.ACP.Send(ctx, acp.SendRequest{
			Session:       session.ID,
			Message:       prompt.Text,
			Contexts:      prompt.Contexts,
			Attachments:   attachments,
			Completion:    acp.CompletionAsync,
			PlanRequested: prompt.PlanRequested,
			GoalRequested: prompt.GoalRequested,
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
	internalQueue, publicQueue := splitQueuedMessages(queue)
	if prompt.IsInternal() {
		if index < 0 || index > len(internalQueue) {
			index = len(internalQueue)
		}
		internalQueue = append(internalQueue[:index], append([]storage.QueuedMessage{prompt.AsInternal()}, internalQueue[index:]...)...)
	} else {
		if index < 0 || index > len(publicQueue) {
			index = len(publicQueue)
		}
		publicQueue = append(publicQueue[:index], append([]storage.QueuedMessage{prompt.AsPublic()}, publicQueue[index:]...)...)
	}
	queue = append(internalQueue, publicQueue...)
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
