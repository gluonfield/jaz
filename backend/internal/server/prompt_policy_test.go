package server

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestNativeGoalSupport(t *testing.T) {
	tests := []struct {
		name    string
		session storage.Session
		want    promptFeatureSupport
	}{
		{
			name: "stored runtime capability supports goal",
			session: storage.Session{
				Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{
					Agent:        acp.AgentCodex,
					SessionID:    "acp-session",
					Capabilities: &storage.RuntimeCapabilities{NativeGoal: true},
				},
			},
			want: promptFeatureSupported,
		},
		{
			name: "catalog codex may negotiate before acp session exists",
			session: storage.Session{
				Runtime:    storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex},
			},
			want: promptFeatureNegotiable,
		},
		{
			name: "created codex session without runtime capability is unsupported",
			session: storage.Session{
				Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{
					Agent:     acp.AgentCodex,
					SessionID: "acp-session",
				},
			},
			want: promptFeatureUnsupported,
		},
		{
			name: "non acp session is unsupported",
			session: storage.Session{
				Runtime: "native",
			},
			want: promptFeatureUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nativeGoalSupport(tt.session); got != tt.want {
				t.Fatalf("nativeGoalSupport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanonicalSessionResponseNativeGoalCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		session    storage.Session
		wantNative bool
		wantNeg    bool
	}{
		{
			name: "prelaunch codex advertises negotiable goal support",
			session: storage.Session{
				Runtime:    storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex},
			},
			wantNeg: true,
		},
		{
			name: "created codex without runtime capability stays unsupported",
			session: storage.Session{
				Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{
					Agent:     acp.AgentCodex,
					SessionID: "acp-session",
				},
			},
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
