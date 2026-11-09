package live

import (
	"fmt"
)

type ClaudeStart struct {
	SDP     string `json:"sdp"`
	Context string `json:"context"`
	Voice   string `json:"voice"`
}

type ClaudeCommand struct {
	Type      string       `json:"type"`
	ID        string       `json:"id,omitempty"`
	Options   *ClaudeStart `json:"options,omitempty"`
	Data      string       `json:"data,omitempty"`
	ToolUseID string       `json:"toolUseId,omitempty"`
	Text      string       `json:"text"`
	IsError   bool         `json:"isError"`
}

type ClaudeStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func (s *Service) PrepareClaude(sdp, chatContext, sessionID string) (ClaudeStart, error) {
	status, name, err := s.prepare(sdp, chatContext, sessionID)
	if err != nil {
		return ClaudeStart{}, err
	}
	if status.Provider != ProviderClaude {
		return ClaudeStart{}, fmt.Errorf("%w: voice provider changed; start voice again", ErrInput)
	}
	return ClaudeStart{
		SDP:     sdp,
		Context: fmt.Sprintf(instructions, name) + "\n\n" + chatContext,
		Voice:   status.Voice,
	}, nil
}
