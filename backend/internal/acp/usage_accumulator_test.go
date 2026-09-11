package acp

import (
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestUsageWriteFailureRetainsSnapshotsAndDeltas(t *testing.T) {
	for _, delta := range []bool{false, true} {
		t.Run(map[bool]string{false: "snapshot", true: "delta"}[delta], func(t *testing.T) {
			var accumulator usageAccumulator
			first := usageReport{ID: "main", Snapshot: storage.Usage{InputTokens: 100, CachedInputTokens: 80, OutputTokens: 10, TotalTokens: 110}, Context: storage.Usage{ContextTokens: 999}}
			if delta {
				first.Delta = first.Snapshot
				first.Snapshot = storage.Usage{}
			}
			failure := errors.New("write failed")
			if written, err := accumulator.record(first, func(storage.Usage) error {
				return failure
			}); written || !errors.Is(err, failure) {
				t.Fatalf("failed write = %v, %v", written, err)
			}
			var saved storage.Usage
			persist := func(usage storage.Usage) error {
				saved = addUsageDelta(saved, usage)
				return nil
			}
			second := usageReport{ID: "side", Auxiliary: true, Snapshot: storage.Usage{InputTokens: 200, CachedInputTokens: 150, OutputTokens: 20, TotalTokens: 220, ContextTokens: 500}}
			if written, err := accumulator.record(second, persist); !written || err != nil {
				t.Fatalf("retry = %v, %v", written, err)
			}
			accumulator.record(first, persist)
			if saved.InputTokens != 300 || saved.CachedInputTokens != 230 || saved.OutputTokens != 30 || saved.TotalTokens != 330 || saved.ContextTokens != 999 {
				t.Fatalf("saved usage = %#v", saved)
			}
		})
	}
}

func TestUsageTurnResetRetainsUnwrittenUsage(t *testing.T) {
	var accumulator usageAccumulator
	report := usageReport{Snapshot: storage.Usage{InputTokens: 100}}
	accumulator.record(report, func(storage.Usage) error {
		return errors.New("write failed")
	})
	accumulator.startTurn()
	var saved storage.Usage
	written, err := accumulator.record(usageReport{}, func(usage storage.Usage) error {
		saved = usage
		return nil
	})
	if !written || err != nil || saved.InputTokens != 100 {
		t.Fatalf("flush after reset = %v, %v, %#v", written, err, saved)
	}
}

func TestUsageWritesPreserveContextArrivalOrder(t *testing.T) {
	var accumulator usageAccumulator
	started := make(chan struct{})
	release := make(chan struct{})
	contexts := make(chan int64, 2)
	go func() {
		accumulator.record(usageReport{Context: storage.Usage{ContextTokens: 100}}, func(usage storage.Usage) error {
			close(started)
			<-release
			contexts <- usage.ContextTokens
			return nil
		})
	}()
	<-started
	go func() {
		accumulator.record(usageReport{Context: storage.Usage{ContextTokens: 50}}, func(usage storage.Usage) error {
			contexts <- usage.ContextTokens
			return nil
		})
	}()
	select {
	case context := <-contexts:
		close(release)
		t.Fatalf("write overtook blocked context update: %d", context)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for _, want := range []int64{100, 50} {
		select {
		case got := <-contexts:
			if got != want {
				t.Fatalf("context write = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("usage write did not finish")
		}
	}
}
