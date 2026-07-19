package app

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type maintenanceResult struct {
	result sqlitestore.SessionEventCompaction
	err    error
}

type fakeMaintenanceStore struct {
	mu      sync.Mutex
	results []maintenanceResult
	pending []bool
	call    int
	calls   chan int
	wake    chan struct{}
}

func (s *fakeMaintenanceStore) CompactNextSessionEvents(context.Context) (sqlitestore.SessionEventCompaction, error) {
	s.mu.Lock()
	s.call++
	call := s.call
	result := maintenanceResult{}
	if len(s.results) > 0 {
		result = s.results[0]
		s.results = s.results[1:]
	}
	s.mu.Unlock()
	s.calls <- call
	return result.result, result.err
}

func (s *fakeMaintenanceStore) HasPendingSessionEventCompaction(context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return false, nil
	}
	pending := s.pending[0]
	s.pending = s.pending[1:]
	return pending, nil
}

func (s *fakeMaintenanceStore) SessionEventCompactionWake() <-chan struct{} {
	return s.wake
}

func TestStorageMaintenanceRetriesAndRescansAfterDrain(t *testing.T) {
	store := &fakeMaintenanceStore{
		results: []maintenanceResult{
			{err: errors.New("transient")},
			{result: sqlitestore.SessionEventCompaction{ThreadID: "thread-1", Removed: 4}},
			{result: sqlitestore.SessionEventCompaction{ThreadID: "thread-1", Removed: 3, Complete: true}},
		},
		pending: []bool{true, false},
		calls:   make(chan int, 8),
		wake:    make(chan struct{}, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		runStorageMaintenance(ctx, store, log.New(io.Discard))
		close(done)
	}()
	for range 5 {
		select {
		case <-store.calls:
		case <-time.After(3 * time.Second):
			cancel()
			t.Fatal("maintenance did not retry after an error and an empty scan")
		}
	}
	select {
	case <-done:
		t.Fatal("maintenance stopped instead of waiting for new work")
	default:
	}
	store.wake <- struct{}{}
	select {
	case <-store.calls:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not resume after new work arrived")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not stop after cancellation")
	}
}

func TestStorageMaintenanceContinuesAcrossBoundedSteps(t *testing.T) {
	store := &fakeMaintenanceStore{
		results: []maintenanceResult{
			{result: sqlitestore.SessionEventCompaction{ThreadID: "thread", Removed: 1}},
			{result: sqlitestore.SessionEventCompaction{ThreadID: "thread", Removed: 1, Complete: true}},
		},
		calls: make(chan int, 3),
		wake:  make(chan struct{}, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runStorageMaintenance(ctx, store, log.New(io.Discard))
		close(done)
	}()
	for range 3 {
		select {
		case <-store.calls:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("maintenance stopped before completing every step")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not stop after cancellation")
	}
}
