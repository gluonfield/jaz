package acp

import (
	"context"
	"encoding/json"
	"fmt"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

type nativeSteerResponse struct {
	Outcome    string             `json:"outcome"`
	Reason     string             `json:"reason"`
	StopReason string             `json:"stopReason"`
	Meta       sessionFailureMeta `json:"_meta"`
}

func (m *Manager) runNativeSteerCall(ctx context.Context, job *jobState, slot steerSlot, prompt acpschema.PromptRequest) {
	peer := m.peer(job.ID)
	if peer == nil {
		m.failSteerCall(slot.done, job, fmt.Errorf("acp peer is not active"))
		return
	}
	m.withACPTranscriptBarrier(job.eventView(), nil)
	raw, err := peer.Call(ctx, string(steerNative), struct {
		SessionID         acpschema.SessionID          `json:"sessionId"`
		Prompt            []acpschema.ContentBlock     `json:"prompt"`
		WaitForCompletion bool                         `json:"waitForCompletion"`
		Meta              map[string]map[string]string `json:"_meta"`
	}{prompt.SessionID, prompt.Prompt, true, map[string]map[string]string{"steering": {"idleBehavior": "promptRequired"}}})
	if err != nil {
		m.failSteerCall(slot.done, job, err)
		return
	}
	var response nativeSteerResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		m.failSteerCall(slot.done, job, err)
		return
	}
	switch response.Outcome {
	case "injected", "startedNewTurn":
		if response.StopReason != "" {
			m.completePromptCall(slot.done, job, response.StopReason, response.Meta.terminal())
			return
		}
	case "promptRequired":
		if response.Reason == "noRunningTurn" {
			if slot.afterSettled != nil {
				select {
				case <-slot.afterSettled:
				case <-slot.done:
					return
				}
			}
			job.mu.Lock()
			if job.turn == nil || job.turn.done != slot.done || job.turn.cancelRequested {
				job.mu.Unlock()
				return
			}
			job.turn.unsent++
			job.mu.Unlock()
			m.runPromptCall(context.Background(), job, slot.done, prompt)
			return
		}
	case "failed":
		m.failSteerCall(slot.done, job, fmt.Errorf("native ACP steering failed; the adapter did not report a cause"))
		return
	}
	m.failSteerCall(slot.done, job, fmt.Errorf("invalid native ACP steering response: outcome %q", response.Outcome))
}
