package acp

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/turnrunner"
)

var _ turnrunner.Runner = (*Manager)(nil)

func (m *Manager) StartTurn(ctx context.Context, req turnrunner.TurnRequest) (turnrunner.TurnResult, error) {
	if req.ThreadID == "" {
		return turnrunner.TurnResult{}, turnrunner.ErrThreadNotFound
	}
	if !storage.HasMessageContent(req.Message, req.Contexts, req.Attachments) {
		return turnrunner.TurnResult{}, turnrunner.ErrEmptyMessage
	}
	job, err := m.Send(ctx, sendRequestFromTurn(req))
	if err != nil {
		return turnrunner.TurnResult{}, mapTurnError(err)
	}
	return turnrunner.TurnResult{
		ThreadID:    job.ID,
		State:       job.State,
		Disposition: turnrunner.DispositionStarted,
	}, nil
}

func (m *Manager) SteerOrQueue(ctx context.Context, req turnrunner.TurnRequest) (turnrunner.TurnResult, error) {
	if req.ThreadID == "" {
		return turnrunner.TurnResult{}, turnrunner.ErrThreadNotFound
	}
	if !storage.HasMessageContent(req.Message, req.Contexts, req.Attachments) {
		return turnrunner.TurnResult{}, turnrunner.ErrEmptyMessage
	}
	job, disposition, err := m.inputOutcome(ctx, sendRequestFromTurn(req), req.Interrupt)
	if err != nil {
		return turnrunner.TurnResult{}, mapTurnError(err)
	}
	return turnrunner.TurnResult{
		ThreadID:    job.ID,
		State:       job.State,
		Disposition: disposition,
	}, nil
}

func (m *Manager) CancelTurn(ctx context.Context, req turnrunner.CancelRequest) (turnrunner.TurnResult, error) {
	if req.ThreadID == "" {
		return turnrunner.TurnResult{}, turnrunner.ErrThreadNotFound
	}
	job, err := m.cancel(ctx, req.ThreadID, cmp.Or(req.Origin, "unknown"), nil)
	if err != nil {
		return turnrunner.TurnResult{}, mapTurnError(err)
	}
	return turnrunner.TurnResult{
		ThreadID:    job.ID,
		State:       job.State,
		Disposition: turnrunner.DispositionCancelled,
	}, nil
}

func sendRequestFromTurn(req turnrunner.TurnRequest) SendRequest {
	return SendRequest{
		Session:       req.ThreadID,
		Message:       req.Message,
		Contexts:      req.Contexts,
		Attachments:   req.Attachments,
		Completion:    CompletionAsync,
		PlanRequested: req.PlanRequested,
		GoalRequested: req.GoalRequested,
		ParentVisible: req.ParentVisible,
		FromUser:      req.FromUser,
		From:          req.From,
		Commitment:    req.Commitment,
	}
}

func mapTurnError(err error) error {
	var active *turnInProgressError
	if errors.As(err, &active) {
		return fmt.Errorf("%w: %v", turnrunner.ErrTurnInProgress, err)
	}
	if errors.Is(err, storage.ErrSessionNotFound) {
		return fmt.Errorf("%w: %v", turnrunner.ErrThreadNotFound, err)
	}
	return err
}
