package storage

import (
	"context"
	"time"
)

type LocalInstruction struct {
	Scope     string
	ScopeID   string
	Kind      string
	BodyMD    string
	Version   string
	UpdatedAt time.Time
}

type LocalInstructionStore interface {
	GetLocalInstruction(ctx context.Context, scope, scopeID, kind string) (LocalInstruction, bool, error)
	ImportLocalInstruction(ctx context.Context, doc LocalInstruction) (LocalInstruction, error)
	SaveLocalInstruction(ctx context.Context, doc LocalInstruction, expectedVersion string) (LocalInstruction, bool, error)
}
