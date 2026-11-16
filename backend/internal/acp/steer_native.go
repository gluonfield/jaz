package acp

import (
	"context"
	"encoding/json"
	"fmt"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

type nativeSteerResponse struct {
	Outcome    string             `json:"outcome"`
	StopReason string             `json:"stopReason"`
	Meta       sessionFailureMeta `json:"_meta"`
}

func (m *Manager) runNativeSteerCall(ctx context.Context, job *jobState, done chan struct{}, prompt acpschema.PromptRequest) {
	peer := m.peer(job.ID)
	if peer == nil {
		m.failSteerCall(done, job, fmt.Errorf("acp peer is not active"))
		return
	}
	m.withACPTranscriptBarrier(job.eventView(), nil)
	raw, err := peer.Call(ctx, string(steerNative), struct {
		SessionID         acpschema.SessionID      `json:"sessionId"`
		Prompt            []acpschema.ContentBlock `json:"prompt"`
		WaitForCompletion bool                     `json:"waitForCompletion"`
	}{prompt.SessionID, prompt.Prompt, true})
	if err != nil {
		m.failSteerCall(done, job, err)
		return
	}
	var response nativeSteerResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		m.failSteerCall(done, job, err)
		return
	}
	switch response.Outcome {
	case "injected", "startedNewTurn":
		if response.StopReason != "" {
			m.completePromptCall(done, job, response.StopReason, response.Meta.terminal())
			return
		}
	case "failed":
		m.failSteerCall(done, job, fmt.Errorf("native ACP steering failed; the adapter did not report a cause"))
		return
	}
	m.failSteerCall(done, job, fmt.Errorf("invalid native ACP steering response: outcome %q", response.Outcome))
}
