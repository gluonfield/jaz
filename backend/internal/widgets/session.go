package widgets

import (
	"context"
	"errors"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
)

type SessionSource interface {
	LoadSession(id string) (storage.Session, error)
}

type LoopSource interface {
	LoadRun(id string) (loops.Run, error)
	LoadLoop(id string) (loops.Loop, error)
}

// SessionPublisher resolves a session back to its loop run so agents can
// publish through session-scoped channels (the MCP tool and the ACP extension
// method).
type SessionPublisher struct {
	Service  *Service
	Sessions SessionSource
	Loops    LoopSource
}

func (p *SessionPublisher) PublishForSession(ctx context.Context, sessionID string, input PublishInput) (Published, error) {
	loop, run, err := p.resolve(sessionID)
	if err != nil {
		return Published{}, err
	}
	return p.Service.Publish(ctx, loop, run.ID, sessionID, input)
}

func (p *SessionPublisher) WidgetSurfaceForSession(sessionID string) bool {
	session, err := p.Sessions.LoadSession(sessionID)
	if err != nil || session.RuntimeRef == nil {
		return false
	}
	return session.SourceType == storage.SourceLoopRun &&
		session.SourceID != "" &&
		visualize.NormalizeSurface(session.RuntimeRef.ArtifactSurface) == visualize.SurfaceWidget
}

var errNotLoopRun = errors.New("publishing a board app is only available to loop runs")

func (p *SessionPublisher) resolve(sessionID string) (loops.Loop, loops.Run, error) {
	session, err := p.Sessions.LoadSession(sessionID)
	if err != nil {
		return loops.Loop{}, loops.Run{}, err
	}
	if session.SourceType != storage.SourceLoopRun || session.SourceID == "" {
		return loops.Loop{}, loops.Run{}, errNotLoopRun
	}
	run, err := p.Loops.LoadRun(session.SourceID)
	if err != nil {
		return loops.Loop{}, loops.Run{}, err
	}
	loop, err := p.Loops.LoadLoop(run.LoopID)
	if err != nil {
		return loops.Loop{}, loops.Run{}, err
	}
	return loop, run, nil
}
