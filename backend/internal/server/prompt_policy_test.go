package server

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestCanonicalSessionResponseNativeGoalCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		session    storage.Session
		wantNative bool
		wantNeg    bool
	}{
		{
			name: "prelaunch codex advertises catalog goal support",
			session: storage.Session{
				Runtime:    storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex},
			},
			wantNative: true,
		},
		{
			name: "created codex without runtime capability advertises catalog support",
			session: storage.Session{
				Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{
					Agent:     acp.AgentCodex,
					SessionID: "acp-session",
				},
			},
			wantNative: true,
		},
		{
			name: "runtime capability advertises negotiated goal support",
			session: storage.Session{
				Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{
					Agent:        acp.AgentCodex,
					SessionID:    "acp-session",
					Capabilities: &storage.RuntimeCapabilities{NativeGoal: true},
				},
			},
			wantNative: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalSessionResponse(tt.session)
			var caps *storage.RuntimeCapabilities
			if got.RuntimeRef != nil {
				caps = got.RuntimeRef.Capabilities
			}
			if (caps != nil && caps.NativeGoal) != tt.wantNative {
				t.Fatalf("native goal = %#v, want %v", caps, tt.wantNative)
			}
			if (caps != nil && caps.NativeGoalNegotiable) != tt.wantNeg {
				t.Fatalf("negotiable goal = %#v, want %v", caps, tt.wantNeg)
			}
		})
	}
}
