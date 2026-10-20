package mobilepush

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (s *Service) Start(ctx context.Context, bus *sessionevents.Bus, report func(error)) <-chan struct{} {
	types := []string{
		"acp", "permission_request", "permission_response", "plan_update", "proposed_plan",
		sessionevents.TypeRoomMessage, sessionevents.TypeSession, sessionevents.TypeTurnFinished,
	}
	stream := bus.Subscribe(ctx, "", types...)
	done := make(chan struct{})
	go func() {
		defer close(done)
		refresh := time.NewTicker(time.Minute)
		defer refresh.Stop()
		check := func(err error) {
			if err != nil && ctx.Err() == nil {
				report(err)
			}
		}
		check(s.refresh(ctx))
		for {
			select {
			case <-ctx.Done():
				return
			case <-refresh.C:
				check(s.refresh(ctx))
			case <-s.changed:
				check(s.refresh(ctx))
			case event, ok := <-stream:
				if !ok {
					if ctx.Err() != nil {
						return
					}
					stream = bus.Subscribe(ctx, "", types...)
					check(s.refresh(ctx))
					continue
				}
				check(s.process(ctx, event))
			}
		}
	}()
	return done
}
