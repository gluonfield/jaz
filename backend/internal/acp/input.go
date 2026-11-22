package acp

import (
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) Input(ctx context.Context, req SteerRequest) (Job, error) {
	return m.input(ctx, req, false)
}

func (m *Manager) SendNow(ctx context.Context, req SendRequest) (Job, error) {
	return m.input(ctx, req, true)
}

func (m *Manager) input(ctx context.Context, req SendRequest, interrupt bool) (Job, error) {
	req.Completion = CompletionAsync
	if req.Commitment != 0 {
		if err := m.store.LinkCommitmentThread(req.From, req.Session, req.Commitment); err != nil {
			return Job{}, err
		}
	}
	session, err := m.store.LoadSession(req.Session)
	if err != nil {
		return Job{}, err
	}
	req.Session = session.ID
	// Card answers already hold the request lock through admission.
	if req.questionAnswer == "" {
		defer m.inputLocks.Lock(session.ID)()
	}
	receipt := inputReceipt(req)
	if receipt.Type != "" {
		reader, ok := m.store.(storage.MessageRecordReader)
		if !ok {
			return Job{}, fmt.Errorf("identified input requires durable message records")
		}
		records, err := reader.LoadMessageRecords(session.ID)
		if err != nil {
			return Job{}, err
		}
		for _, record := range records {
			for _, block := range record.Blocks {
				if record.Role == "user" && block.Type == receipt.Type && block.ID == receipt.ID && block.RequestID == receipt.RequestID {
					// Persistence acknowledges admission, even when a crash left
					// provider dispatch uncertain. A retry must not dispatch again.
					return m.Status(session.ID)
				}
			}
		}
	}
	if m.ThreadStarted != nil {
		m.ThreadStarted(session, req.Message)
	}
	for {
		if err := ctx.Err(); err != nil {
			return Job{}, err
		}
		job, err := m.Send(ctx, req)
		var active *turnInProgressError
		if !errors.As(err, &active) {
			return job, err
		}
		job, err = m.Steer(ctx, req)
		switch {
		case errors.Is(err, errTurnEnded):
		case errors.Is(err, errPromptPending):
			// The message joins the turn once its next prompt reaches the
			// agent, or starts a new one if the turn ends first, even if the
			// caller, such as a voice call, has gone by then.
			ctx = context.WithoutCancel(ctx)
			if err := m.waitPromptSent(ctx, req.Session); err != nil {
				return Job{}, err
			}
		case interrupt && (errors.Is(err, ErrSteeringUnsupported) || errors.Is(err, errPlanChange)):
			if active.done != nil {
				if _, err := m.cancel(ctx, req.Session, "user_input", active.done); err != nil {
					return Job{}, err
				}
			}
		default:
			return job, err
		}
	}
}

func inputReceipt(req SendRequest) storage.Block {
	if req.questionAnswer != "" {
		return storage.Block{Type: storage.BlockTypeQuestionAnswer, RequestID: req.questionAnswer}
	}
	for _, context := range storage.NormalizeMessageContexts(req.Contexts) {
		if context.Type == storage.ContextTypeVoice && context.ID != "" && context.RequestID != "" {
			return storage.Block{Type: storage.BlockTypeVoiceContext, ID: context.ID, RequestID: context.RequestID}
		}
	}
	return storage.Block{}
}

func (m *Manager) waitPromptSent(ctx context.Context, session string) error {
	job, err := m.job(session)
	if err != nil {
		return err
	}
	return job.waitFirstPromptSent(ctx)
}
