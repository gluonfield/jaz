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
		if !errors.Is(err, errTurnEnded) {
			return job, err
		}
	}
}
