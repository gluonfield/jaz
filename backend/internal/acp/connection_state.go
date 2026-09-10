package acp

import (
	"context"
	"encoding/json"
	"sync"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type connectionState struct {
	mu      sync.Mutex
	job     *jobState
	auth    *sessionevents.AgentAuthIdentity
	notices []string
	initial json.RawMessage
	pending []sessionNotification
}

type sessionNotification struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

func (s *connectionState) configResponse(sessionID acpschema.SessionID, raw json.RawMessage) {
	if !parseSessionConfigOptions(raw).configOptionsPresent {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, sessionNotification{SessionID: string(sessionID), Update: raw})
}

func (s *connectionState) capabilities(raw json.RawMessage) {
	var response struct {
		Capabilities struct {
			Meta struct {
				Jaz struct {
					Notices []string `json:"notices"`
				} `json:"jaz"`
			} `json:"_meta"`
		} `json:"agentCapabilities"`
	}
	if json.Unmarshal(raw, &response) == nil {
		s.notices = response.Capabilities.Meta.Jaz.Notices
	}
}

func (s *connectionState) handler(m *Manager, next jsonrpc.Handler) jsonrpc.Handler {
	return jsonrpc.HandlerFunc(func(ctx context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		if req.Method == "_auth/status_update" {
			var note struct {
				Auth *sessionevents.AgentAuthIdentity `json:"authStatus"`
			}
			if json.Unmarshal(req.Params, &note) != nil || note.Auth == nil {
				return nil, jsonrpc.InvalidParams("invalid auth status", nil)
			}
			s.mu.Lock()
			s.auth = note.Auth
			if s.job != nil {
				s.job.mu.Lock()
				s.job.agentSession.Auth = note.Auth
				s.job.mu.Unlock()
				m.publishAgentSession(s.job)
			}
			s.mu.Unlock()
			return jsonrpc.EncodeResult(struct{}{})
		}
		if req.Method == "session/update" {
			var note sessionNotification
			if json.Unmarshal(req.Params, &note) == nil {
				var update struct {
					Kind string `json:"sessionUpdate"`
				}
				if json.Unmarshal(note.Update, &update) == nil && sessionMetadataUpdate(update.Kind) {
					s.mu.Lock()
					if s.job == nil {
						s.pending = append(s.pending, note)
					} else if s.job.ACPSession == note.SessionID {
						m.applyUpdate(note.SessionID, note.Update)
					}
					s.mu.Unlock()
					return jsonrpc.EncodeResult(struct{}{})
				}
			}
		}
		return next.HandleJSONRPC(ctx, req)
	})
}

func (s *connectionState) attach(m *Manager, job *jobState, cfg AgentConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.job = job
	job.mu.Lock()
	job.agentSession.Auth = s.auth
	job.agentSession.Notices = s.notices
	job.mu.Unlock()
	m.applySessionControls(job, s.initial)
	for _, note := range s.pending {
		if note.SessionID == job.ACPSession {
			m.applyUpdate(note.SessionID, note.Update)
		}
	}
	rememberConfiguredChoices(job, cfg)
	s.initial = nil
	s.pending = nil
	m.publishAgentSession(job)
}

func sessionMetadataUpdate(kind string) bool {
	switch kind {
	case "config_option_update", "available_commands_update", "async_task_spawned", "async_task_progress", "async_task_state_update":
		return true
	default:
		return false
	}
}
