package mobilepush

import "context"

// Subscribe streams the alerts decided from now on until ctx ends. A reader
// that falls behind misses alerts rather than holding up delivery.
func (s *Service) Subscribe(ctx context.Context) <-chan Alert {
	alerts := make(chan Alert, 16)
	s.listenMu.Lock()
	s.listeners[alerts] = struct{}{}
	s.listenMu.Unlock()
	go func() {
		<-ctx.Done()
		s.listenMu.Lock()
		delete(s.listeners, alerts)
		close(alerts)
		s.listenMu.Unlock()
	}()
	return alerts
}

func (s *Service) broadcast(alert Alert) {
	s.listenMu.Lock()
	defer s.listenMu.Unlock()
	for listener := range s.listeners {
		select {
		case listener <- alert:
		default:
		}
	}
}

func (s *Service) listening() bool {
	s.listenMu.Lock()
	defer s.listenMu.Unlock()
	return len(s.listeners) > 0
}
