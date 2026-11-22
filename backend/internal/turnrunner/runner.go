package turnrunner

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
)

var (
	ErrEmptyMessage        = errors.New("message is required")
	ErrThreadNotFound      = errors.New("thread not found")
	ErrTurnInProgress      = errors.New("turn is already running")
	ErrSteeringUnsupported = errors.New("steering unsupported")
)

type Disposition string

const (
	DispositionStarted     Disposition = "started"
	DispositionSteered     Disposition = "steered"
	DispositionQueued      Disposition = "queued"
	DispositionInterrupted Disposition = "interrupted"
	DispositionCancelled   Disposition = "cancelled"
)

type TurnRequest struct {
	ThreadID        string                   `json:"thread_id"`
	ActorUserID     string                   `json:"actor_user_id,omitempty"`
	ClientRequestID string                   `json:"client_request_id,omitempty"`
	Message         string                   `json:"message"`
	Contexts        []storage.MessageContext `json:"contexts,omitempty"`
	Attachments     []storage.Attachment     `json:"attachments,omitempty"`
	PlanRequested   bool                     `json:"plan_requested,omitempty"`
	GoalRequested   bool                     `json:"goal_requested,omitempty"`
	ParentVisible   bool                     `json:"parent_visible,omitempty"`
	FromUser        bool                     `json:"from_user,omitempty"`
	From            string                   `json:"from,omitempty"`
	Commitment      int64                    `json:"commitment,omitempty"`
	Interrupt       bool                     `json:"interrupt,omitempty"`
}

type CancelRequest struct {
	ThreadID    string `json:"thread_id"`
	ActorUserID string `json:"actor_user_id,omitempty"`
	Origin      string `json:"origin,omitempty"`
}

type TurnResult struct {
	ThreadID    string      `json:"thread_id"`
	TurnID      string      `json:"turn_id,omitempty"`
	TurnEpoch   int64       `json:"turn_epoch,omitempty"`
	State       string      `json:"state,omitempty"`
	Disposition Disposition `json:"disposition"`
}

// Runner is the transport-neutral turn execution contract shared by local ACP
// sessions and Jaz Cloud's durable Postgres/worker queue.
type Runner interface {
	StartTurn(ctx context.Context, req TurnRequest) (TurnResult, error)
	SteerOrQueue(ctx context.Context, req TurnRequest) (TurnResult, error)
	CancelTurn(ctx context.Context, req CancelRequest) (TurnResult, error)
}
