package acp

import (
	"context"
	"errors"
)

func (m *Manager) Input(ctx context.Context, req SteerRequest) (Job, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Job{}, err
		}
		job, err := m.Send(ctx, SendRequest{
			Session: req.Session, Message: req.Message, Contexts: req.Contexts, Attachments: req.Attachments,
			Completion: CompletionAsync, GoalRequested: req.GoalRequested, ParentVisible: req.ParentVisible,
		})
		var active *turnInProgressError
		if !errors.As(err, &active) {
			return job, err
		}
		job, err = m.Steer(ctx, req)
		if errors.Is(err, errPromptPending) {
			// The message joins the turn once its next prompt reaches the
			// agent, or starts a new one if the turn ends first.
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
