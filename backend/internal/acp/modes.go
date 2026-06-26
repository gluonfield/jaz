package acp

import (
	"context"
	"fmt"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

func modeStateFromACP(state *acpschema.SessionModeState) ModeState {
	if state == nil {
		return ModeState{}
	}
	out := ModeState{
		CurrentModeID: string(state.CurrentModeID),
		PlanModeID:    planModeID(state.AvailableModes),
	}
	out.AvailableModes = make([]ModeSnapshot, 0, len(state.AvailableModes))
	for _, mode := range state.AvailableModes {
		out.AvailableModes = append(out.AvailableModes, ModeSnapshot{
			ID:          string(mode.ID),
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return out
}

func planModeID(modes []acpschema.SessionMode) string {
	for _, mode := range modes {
		if string(mode.ID) == "plan" {
			return string(mode.ID)
		}
	}
	for _, mode := range modes {
		text := strings.ToLower(string(mode.ID) + " " + mode.Name)
		if strings.Contains(text, "plan") {
			return string(mode.ID)
		}
	}
	return ""
}

func (m *Manager) prepareModeForTurn(ctx context.Context, job *jobState, planRequested bool) error {
	job.mu.RLock()
	modes := job.Modes.Clone()
	job.mu.RUnlock()

	if planRequested {
		if modes.PlanModeID == "" {
			return fmt.Errorf("acp session %s does not expose plan mode", job.Slug)
		}
		return m.applyTurnMode(ctx, job, modes.PlanModeID)
	}
	target := baselineModeID(job.ACPAgent, modes)
	if modes.PlanModeID != "" {
		// session/load can report the baseline mode while the provider thread still carries stale Plan settings.
		return m.applyTurnMode(ctx, job, target)
	}
	return m.ensureTurnMode(ctx, job, target)
}

func (m *Manager) ensureTurnMode(ctx context.Context, job *jobState, target string) error {
	job.mu.RLock()
	current := job.Modes.CurrentModeID
	job.mu.RUnlock()
	if target == current {
		return nil
	}
	return m.applyTurnMode(ctx, job, target)
}

func (m *Manager) applyTurnMode(ctx context.Context, job *jobState, target string) error {
	if target == "" {
		return nil
	}

	job.mu.RLock()
	acpSessionID := job.ACPSession
	jobID := job.ID
	job.mu.RUnlock()

	peer := m.peer(jobID)
	if peer == nil {
		if m.configuredLocal(job.ACPAgent) {
			job.mu.Lock()
			job.Modes.CurrentModeID = target
			job.mu.Unlock()
			return nil
		}
		return fmt.Errorf("acp peer is not active")
	}
	if err := m.setSessionMode(ctx, peer, acpschema.SessionID(acpSessionID), target); err != nil {
		return err
	}
	job.mu.Lock()
	job.Modes.CurrentModeID = target
	job.mu.Unlock()
	return nil
}

func (m *Manager) setSessionMode(ctx context.Context, peer *jsonrpc.Peer, sessionID acpschema.SessionID, modeID string) error {
	_, err := peer.Call(ctx, acpschema.AgentMethodSessionSetMode, acpschema.SetSessionModeRequest{
		SessionID: sessionID,
		ModeID:    acpschema.SessionModeID(modeID),
	})
	if err != nil {
		return fmt.Errorf("set acp session %q mode: %w", modeID, err)
	}
	return nil
}
