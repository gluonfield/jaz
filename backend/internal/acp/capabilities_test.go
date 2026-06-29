package acp

import (
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestEffectiveRuntimeCapabilitiesMergesCatalogSupport(t *testing.T) {
	tests := []struct {
		name       string
		agent      string
		caps       *storage.RuntimeCapabilities
		wantNative bool
		wantNeg    bool
	}{
		{
			name:       "catalog native goal wins over weaker stored negotiable support",
			agent:      AgentCodex,
			caps:       &storage.RuntimeCapabilities{NativeGoalNegotiable: true},
			wantNative: true,
		},
		{
			name:    "non catalog negotiable support is preserved",
			agent:   AgentClaude,
			caps:    &storage.RuntimeCapabilities{NativeGoalNegotiable: true},
			wantNeg: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveRuntimeCapabilities(tt.agent, tt.caps)
			if (got != nil && got.NativeGoal) != tt.wantNative {
				t.Fatalf("native goal = %#v, want %v", got, tt.wantNative)
			}
			if (got != nil && got.NativeGoalNegotiable) != tt.wantNeg {
				t.Fatalf("negotiable goal = %#v, want %v", got, tt.wantNeg)
			}
		})
	}
}
