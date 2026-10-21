package acp

import (
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) Input(ctx context.Context, req SteerRequest) (Job, error) {
	for _, voice := range storage.NormalizeMessageContexts(req.Contexts) {
		if voice.Type != storage.ContextTypeVoice || voice.ID == "" || voice.RequestID == "" {
			continue
		}
		session, err := m.store.LoadSession(req.Session)
		if err != nil {
			return Job{}, err
		}
		req.Session = session.ID
		defer m.inputLocks.Lock(session.ID)()
		reader, ok := m.store.(storage.MessageRecordReader)
		if !ok {
			return Job{}, fmt.Errorf("voice input requires durable message records")
		}
		records, err := reader.LoadMessageRecords(session.ID)
		if err != nil {
			return Job{}, err
		}
		for _, record := range records {
			for _, block := range record.Blocks {
				if record.Role == "user" && block.Type == storage.BlockTypeVoiceContext && block.ID == voice.ID && block.RequestID == voice.RequestID {
					// Persistence acknowledges admission, even when a crash left
					// provider dispatch uncertain. A retry must not dispatch again.
					return m.Status(session.ID)
				}
			}
		}
		break
	}
	for {
		if err := ctx.Err(); err != nil {
			return Job{}, err
		}
		job, err := m.Send(ctx, SendRequest{
			Session: req.Session, Message: req.Message, Contexts: req.Contexts, Attachments: req.Attachments,
			Completion: CompletionAsync, GoalRequested: req.GoalRequested, ParentVisible: req.ParentVisible, FromUser: req.FromUser,
		})
		var active *turnInProgressError
		if !errors.As(err, &active) {
			return job, err
		}
		job, err = m.Steer(ctx, req)
		if errors.Is(err, errPromptPending) {
			// The message joins the turn once its next prompt reaches the
			// agent, or starts a new one if the turn ends first, even if the
			// caller, such as a voice call, has gone by then.
			ctx = context.WithoutCancel(ctx)
			if err := m.waitPromptSent(ctx, req.Session); err != nil {
				return Job{}, err
			}
			continue
		}
		if !errors.Is(err, errTurnEnded) {
			return job, err
		}
	}
}

func (m *Manager) waitPromptSent(ctx context.Context, session string) error {
	job, err := m.job(session)
	if err != nil {
		return err
	}
	return job.waitFirstPromptSent(ctx)
}
