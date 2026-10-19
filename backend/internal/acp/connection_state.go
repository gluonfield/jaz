package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

// Grok answers initialize and session/new from a built-in model list while it
// renews an expired login, then announces its real list with this notification.
const grokModelsUpdateMethod = "_x.ai/models/update"

const advertisedModelWait = 5 * time.Second

type connectionState struct {
	mu              sync.Mutex
	job             *jobState
	auth            *sessionevents.AgentAuthIdentity
	notices         []string
	initial         json.RawMessage
	pending         []sessionNotification
	initModels      sessionModelState
	modelsAnnounced chan struct{}
	announceModels  sync.Once
}

func newConnectionState() *connectionState {
	return &connectionState{modelsAnnounced: make(chan struct{})}
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

// initialized records what the agent's initialize result says about it.
func (s *connectionState) initialized(raw json.RawMessage) {
	var response struct {
		Capabilities struct {
			Meta struct {
				Jaz struct {
					Notices []string `json:"notices"`
				} `json:"jaz"`
			} `json:"_meta"`
		} `json:"agentCapabilities"`
		Meta struct {
			ModelState advertisedModelList `json:"modelState"`
		} `json:"_meta"`
	}
	if json.Unmarshal(raw, &response) == nil {
		s.notices = response.Capabilities.Meta.Jaz.Notices
		s.initModels = response.Meta.ModelState.state()
	}
}

// awaitAdvertisedModel holds a start whose model the agent's initialize list
// refuses until the agent announces its list again. Without an announcement
// it refuses the model before session/new, which would answer from that list.
func (s *connectionState) awaitAdvertisedModel(ctx context.Context, closed <-chan struct{}, agent string, cfg AgentConfig) error {
	policy := agentPolicyForAgent(agent)
	_, err := policy.sessionModelToSend(agent, policy.sessionConfigModel(cfg), s.initModels, cfg.ModelRequested)
	if err == nil {
		return nil
	}
	timer := time.NewTimer(advertisedModelWait)
	defer timer.Stop()
	select {
	case <-s.modelsAnnounced:
		return nil
	case <-closed:
		// session/new then reports the exit along with the agent's stderr.
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("%w (the agent did not announce its model list within %s)", err, advertisedModelWait)
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
		if req.Method == grokModelsUpdateMethod {
			s.announceModels.Do(func() { close(s.modelsAnnounced) })
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
