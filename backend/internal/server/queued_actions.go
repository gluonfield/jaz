package server

import (
	"context"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type queuedActionSpec struct {
	Timeout     time.Duration
	Async       bool
	RequiresCwd bool
	CanStart    func(*Server, storage.Session) bool
	Run         func(*Server, context.Context, storage.Session) error
	AfterFinish func(*Server, string)
}

var queuedActionSpecs = map[storage.QueuedAction]queuedActionSpec{
	storage.QueuedActionArchive: {
		Run: func(s *Server, _ context.Context, session storage.Session) error {
			_, err := s.setSessionArchivedState(session.ID, true)
			return err
		},
		AfterFinish: func(s *Server, _ string) { s.pruneManagedWorktreesSoon() },
	},
	storage.QueuedActionUnarchive: {
		Run: func(s *Server, _ context.Context, session storage.Session) error {
			_, err := s.setSessionArchivedState(session.ID, false)
			return err
		},
	},
	storage.QueuedActionCompact: {
		Async: true,
		CanStart: func(s *Server, session storage.Session) bool {
			return s.ACP != nil && sessionSupportsCompact(session)
		},
		Run: func(s *Server, ctx context.Context, session storage.Session) error {
			return s.startQueuedCompact(ctx, session)
		},
	},
	storage.QueuedActionRepoCommit: {
		Timeout:     30 * time.Second,
		RequiresCwd: true,
		Run: func(s *Server, ctx context.Context, session storage.Session) error {
			_, err := s.commitSessionRepo(ctx, session, "")
			return err
		},
	},
	storage.QueuedActionRepoPush: {
		Timeout:     60 * time.Second,
		RequiresCwd: true,
		Run: func(s *Server, ctx context.Context, session storage.Session) error {
			_, err := s.pushSessionRepo(ctx, session)
			return err
		},
	},
	storage.QueuedActionRepoMerge: {
		Timeout:     30 * time.Second,
		RequiresCwd: true,
		Run: func(s *Server, ctx context.Context, session storage.Session) error {
			_, _, err := s.mergeSessionRepo(ctx, session, "")
			return err
		},
	},
	storage.QueuedActionRepoMergeFromMain: {
		Timeout:     30 * time.Second,
		RequiresCwd: true,
		Run: func(s *Server, ctx context.Context, session storage.Session) error {
			_, err := s.mergeFromMainSessionRepo(ctx, session, "")
			return err
		},
	},
	storage.QueuedActionRepoRestoreWorktree: {
		Timeout: 60 * time.Second,
		CanStart: func(s *Server, session storage.Session) bool {
			_, ok := s.managedWorktree(session)
			return ok
		},
		Run: func(s *Server, ctx context.Context, session storage.Session) error {
			_, err := s.restoreSessionWorktree(ctx, session)
			return err
		},
	},
}

func (s *Server) canStartQueuedAction(session storage.Session, action storage.QueuedAction) bool {
	spec, ok := queuedActionSpecs[action]
	if !ok {
		return false
	}
	if spec.CanStart != nil && !spec.CanStart(s, session) {
		return false
	}
	if spec.RequiresCwd && optionalCwd(session) == "" {
		return false
	}
	return true
}

func (s *Server) startQueuedAction(ctx context.Context, session storage.Session, prompt storage.QueuedMessage) (bool, error) {
	spec, ok := queuedActionSpecs[prompt.Action]
	if !ok {
		return false, fmt.Errorf("unknown queued action %q", prompt.Action)
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	if spec.RequiresCwd {
		if err := s.ensureManagedWorktree(ctx, session); err != nil {
			return spec.Async, err
		}
	}
	return spec.Async, spec.Run(s, ctx, session)
}

func (s *Server) startQueuedCompact(ctx context.Context, session storage.Session) error {
	if session.Runtime != storage.RuntimeACP {
		return fmt.Errorf("compact is only available for acp sessions")
	}
	if !sessionSupportsCompact(session) {
		return fmt.Errorf("compact is not available for this session")
	}
	if s.ACP == nil {
		return fmt.Errorf("acp manager is not configured")
	}
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		return err
	}
	if _, err := s.ACP.Compact(ctx, acp.CompactRequest{Session: session.ID}); err != nil {
		return acpSendError(session, err)
	}
	return nil
}

func (s *Server) finishQueuedAction(sessionID string, action storage.QueuedAction) {
	s.setSessionStatus(storage.Session{ID: sessionID}, storage.StatusIdle)
	s.publishMessagesChanged(sessionID)
	s.publishSessionChanged(sessionID)
	if spec, ok := queuedActionSpecs[action]; ok && spec.AfterFinish != nil {
		spec.AfterFinish(s, sessionID)
	}
	s.drainQueueSoon(sessionID)
}
