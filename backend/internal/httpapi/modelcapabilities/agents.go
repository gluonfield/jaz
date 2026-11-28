package modelcapabilities

import (
	"context"
	"net/http"
	"sync"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi"
)

type AgentModelSource interface {
	DiscoverModels(context.Context, string) (acp.AgentModelCatalog, error)
}

type agentModelsFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	catalog acp.AgentModelCatalog
	err     error
}

// NewAgentModelsHandler serves per-agent model catalogs and coalesces
// concurrent discovery calls for the same agent so parallel UI mounts do not
// spawn duplicate CLI processes.
func NewAgentModelsHandler(source AgentModelSource) http.Handler {
	var mu sync.Mutex
	inFlight := map[string]*agentModelsFlight{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := acp.CanonicalAgentName(r.PathValue("agent"))
		mu.Lock()
		flight := inFlight[agent]
		if flight == nil {
			ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
			flight = &agentModelsFlight{
				done:   make(chan struct{}),
				cancel: cancel,
			}
			inFlight[agent] = flight
			go func(current *agentModelsFlight) {
				defer cancel()
				current.catalog, current.err = source.DiscoverModels(ctx, agent)
				mu.Lock()
				if inFlight[agent] == current {
					delete(inFlight, agent)
				}
				mu.Unlock()
				close(current.done)
			}(flight)
		}
		flight.waiters++
		mu.Unlock()

		var catalog acp.AgentModelCatalog
		var err error
		select {
		case <-flight.done:
			catalog, err = flight.catalog, flight.err
		case <-r.Context().Done():
			mu.Lock()
			flight.waiters--
			if flight.waiters == 0 {
				if inFlight[agent] == flight {
					delete(inFlight, agent)
				}
				flight.cancel()
			}
			mu.Unlock()
			httpapi.WriteError(w, http.StatusBadGateway, r.Context().Err())
			return
		}
		if err != nil {
			httpapi.WriteError(w, http.StatusBadGateway, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, catalog)
	})
}
