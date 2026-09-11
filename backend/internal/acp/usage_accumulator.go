package acp

import (
	"sync"

	"github.com/wins/jaz/backend/internal/storage"
)

type usageScope struct {
	usage     storage.Usage
	lastDelta storage.Usage
}

type usageAccumulator struct {
	mu      sync.Mutex
	scopes  map[string]usageScope
	pending storage.Usage
}

func (a *usageAccumulator) startTurn() {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.scopes, "")
}

func (a *usageAccumulator) record(report usageReport, persist func(storage.Usage) error) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !report.IsZero() {
		previous := a.scopes[report.ID]
		next := usageScope{
			usage:     mergeUsageSnapshot(previous.usage, report.Snapshot),
			lastDelta: report.Delta,
		}
		if !report.Delta.IsZero() && report.Delta != previous.lastDelta {
			next.usage = addUsageDelta(next.usage, report.Delta)
		}
		write := mergeUsageContext(usageDelta(previous.usage, next.usage), report.Context)
		if report.Auxiliary {
			write.ContextTokens = 0
			write.ContextWindowTokens = 0
		}
		a.pending = addUsageDelta(a.pending, write)
		next.usage.ContextTokens = 0
		next.usage.ContextWindowTokens = 0
		if a.scopes == nil {
			a.scopes = make(map[string]usageScope)
		}
		a.scopes[report.ID] = next
	}
	if a.pending.IsZero() {
		return false, nil
	}
	if err := persist(a.pending); err != nil {
		return false, err
	}
	a.pending = storage.Usage{}
	return true, nil
}
