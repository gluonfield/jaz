package acp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestHydrationToolProgressIsBoundedAndDropsPayloads(t *testing.T) {
	now := time.Now().UTC()
	job := Job{}
	for i := range 20 {
		job.ToolCalls = append(job.ToolCalls, sessionevents.ACPToolCall{
			ID: fmt.Sprintf("tool-%d", i), Kind: "execute", Status: "in_progress", UpdatedAt: now.Add(time.Duration(i) * time.Second),
			Title: "secret-title", RawInput: map[string]any{"command": "secret-argument"},
		})
	}
	job.ToolCalls = append(job.ToolCalls, sessionevents.ACPToolCall{ID: "finished", Kind: "read", Status: "completed", UpdatedAt: now.Add(time.Minute)})
	view := HydrationViewFromJob(job)
	if len(view.ToolProgress) != 9 || view.ToolProgress[0].ID != "tool-19" || view.ToolProgress[8].ID != "finished" {
		t.Fatalf("bounded progress = %#v", view.ToolProgress)
	}
	roundtrip := view.Job()
	if len(roundtrip.ToolCalls) != 0 || len(roundtrip.ToolProgress) != 9 {
		t.Fatalf("hydration restored tool payloads or lost progress: %#v", roundtrip)
	}
	view.ToolProgress[0].ID = "mutated"
	if roundtrip.ToolProgress[0].ID == "mutated" {
		t.Fatal("hydration progress aliases another snapshot")
	}
	job.ToolCalls = []sessionevents.ACPToolCall{{ID: strings.Repeat("x", 1000), Kind: "secret-kind", Status: "secret-status"}}
	progress := HydrationViewFromJob(job).ToolProgress[0]
	if len(progress.ID) != 128 || progress.Kind != "" || progress.Status != "" {
		t.Fatalf("unbounded metadata = %#v", progress)
	}
}
