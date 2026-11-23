package instructions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

type Scope string

const (
	ScopeOrg   Scope = "org"
	ScopeSpace Scope = "space"
	ScopeUser  Scope = "user"
	ScopeBot   Scope = "bot"
)

type Kind string

const (
	KindAgents    Kind = "agents"
	KindBots      Kind = "bots"
	KindMemory    Kind = "memory"
	KindLongTerm  Kind = "long_term"
	KindShortTerm Kind = "short_term"
)

var (
	ErrConflict         = errors.New("the file changed since it was read")
	ErrInvalidKey       = errors.New("invalid instruction key")
	ErrUnsupportedScope = errors.New("instruction scope is not supported by this store")
)

type Key struct {
	OrgID   string `json:"org_id,omitempty"`
	Scope   Scope  `json:"scope"`
	ScopeID string `json:"scope_id,omitempty"`
	Kind    Kind   `json:"kind"`
}

func (k Key) Validate() error {
	switch k.Scope {
	case ScopeOrg:
		if k.Kind != KindAgents {
			return ErrInvalidKey
		}
	case ScopeSpace:
		if k.ScopeID == "" || k.Kind != KindAgents {
			return ErrInvalidKey
		}
	case ScopeUser:
		if k.Kind != KindAgents && k.Kind != KindBots && k.Kind != KindLongTerm && k.Kind != KindShortTerm {
			return ErrInvalidKey
		}
	case ScopeBot:
		if k.ScopeID == "" || (k.Kind != KindBots && k.Kind != KindMemory) {
			return ErrInvalidKey
		}
	default:
		return ErrInvalidKey
	}
	return nil
}

type Document struct {
	Key       Key       `json:"key"`
	Content   string    `json:"content"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

type Store interface {
	Get(ctx context.Context, key Key) (Document, error)
	Save(ctx context.Context, key Key, content, expectedVersion string) (Document, error)
}

// Digest returns the 16-hex SHA-256 version prefix used for optimistic
// concurrency checks across local files and Jaz Cloud PostgreSQL instructions.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
