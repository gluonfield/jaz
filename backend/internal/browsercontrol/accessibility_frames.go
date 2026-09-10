package browsercontrol

import (
	"context"
	"fmt"
)

type axFrameTree struct {
	Frame    axFrame       `json:"frame"`
	Children []axFrameTree `json:"childFrames"`
}

func (p *browserPage) accessibilityFrames(ctx context.Context) ([]axFrame, error) {
	var sessions []struct {
		SessionID string      `json:"sessionId"`
		Tree      axFrameTree `json:"frameTree"`
	}
	if err := p.conn.call(ctx, "Jaz.frames", nil, &sessions); err != nil {
		return nil, err
	}
	frames := map[string]axFrame{}
	var order []string
	var visit func(axFrameTree, string)
	visit = func(tree axFrameTree, sessionID string) {
		frame := tree.Frame
		frame.SessionID = sessionID
		if _, exists := frames[frame.ID]; !exists {
			order = append(order, frame.ID)
		}
		frames[frame.ID] = frame
		for _, child := range tree.Children {
			visit(child, sessionID)
		}
	}
	for _, session := range sessions {
		visit(session.Tree, session.SessionID)
	}
	result := make([]axFrame, 0, len(order))
	for _, id := range order {
		frame := frames[id]
		if frame.ParentID != "" {
			parent, ok := frames[frame.ParentID]
			if !ok {
				return nil, fmt.Errorf("accessibility frame %s has no parent frame", id)
			}
			var owner struct {
				BackendID int64 `json:"backendNodeId"`
			}
			if err := p.conn.callSession(ctx, parent.SessionID, "DOM.getFrameOwner", map[string]any{"frameId": id}, &owner); err != nil {
				return nil, err
			}
			frame.OwnerID = owner.BackendID
		}
		result = append(result, frame)
	}
	return result, nil
}
