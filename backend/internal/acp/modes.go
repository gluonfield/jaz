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

func grokFallbackModes() *acpschema.SessionModeState {
	return &acpschema.SessionModeState{
		CurrentModeID: acpschema.SessionModeID("ask"),
		AvailableModes: []acpschema.SessionMode{
			{
				ID:          acpschema.SessionModeID("ask"),
				Name:        "Ask",
				Description: "Request permission before tool calls",
			},
			{
				ID:          acpschema.SessionModeID("plan"),
				Name:        "Plan",
				Description: "Plan before making changes",
			},
			{
				ID:          acpschema.SessionModeID("always-approve"),
				Name:        "Always Approve",
				Description: "Run tool calls without permission prompts",
			},
		},
	}
}

func (m *Manager) prepareModeForTurn(ctx context.Context, job *Job, planRequested bool) error {
	job.mu.RLock()
	modes := job.Modes.Clone()
	acpSessionID := job.ACPSession
	jobID := job.ID
	job.mu.RUnlock()

	target := modes.ExecutionModeID
	if planRequested {
		if modes.PlanModeID == "" {
			return fmt.Errorf("acp session %s does not expose plan mode", job.Slug)
		}
		target = modes.PlanModeID
	}
	if target == "" || target == modes.CurrentModeID {
		return nil
	}
	peer := m.peer(jobID)
	if peer == nil {
		return fmt.Errorf("acp peer is not active")
	}
	if err := m.setSessionMode(ctx, peer, acpschema.SessionID(acpSessionID), target); err != nil {
		return err
	}
	job.mu.Lock()
	job.Modes.CurrentModeID = target
	if !planRequested && job.Modes.ExecutionModeID == "" {
		job.Modes.ExecutionModeID = target
	}
	job.mu.Unlock()
	return nil
}

func (m *Manager) restoreExecutionMode(ctx context.Context, job *Job) error {
	return m.prepareModeForTurn(ctx, job, false)
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
