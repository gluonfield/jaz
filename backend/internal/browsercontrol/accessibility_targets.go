package browsercontrol

import (
	"context"
	"fmt"
	"strings"
)

func (p *browserPage) evalRef(ctx context.Context, ref, expression string, out any) error {
	if !strings.HasPrefix(ref, "ax:") {
		return p.eval(ctx, expression, out)
	}
	target, ok := p.ax.targets[ref]
	if !ok {
		return fmt.Errorf("accessibility index %s is unavailable; call getAXState() again", ref)
	}
	frames, err := p.accessibilityFrames(ctx)
	if err != nil {
		return err
	}
	var current *axFrame
	for _, frame := range frames {
		if frame.ID == target.Frame.ID && frame.LoaderID == target.Frame.LoaderID && frame.SessionID == target.Frame.SessionID {
			current = &frame
			break
		}
	}
	if current == nil {
		return fmt.Errorf("accessibility index %s belongs to a previous document; call getAXState() again", ref)
	}
	var world struct {
		ID int64 `json:"executionContextId"`
	}
	if err := p.conn.callSession(ctx, current.SessionID, "Page.createIsolatedWorld", map[string]any{"frameId": current.ID, "worldName": "jaz-browser-control"}, &world); err != nil {
		return err
	}
	var resolved struct {
		Object struct {
			ID string `json:"objectId"`
		} `json:"object"`
	}
	if err := p.conn.callSession(ctx, current.SessionID, "DOM.resolveNode", map[string]any{"backendNodeId": target.BackendID, "executionContextId": world.ID}, &resolved); err != nil {
		return fmt.Errorf("accessibility target is no longer available: %w", err)
	}
	defer p.conn.callSession(ctx, current.SessionID, "Runtime.releaseObject", map[string]any{"objectId": resolved.Object.ID}, nil)
	var result cdpEvaluation
	if err := p.conn.callSession(ctx, current.SessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId": resolved.Object.ID,
		"functionDeclaration": `function(ref, expression) {
  const element = this.nodeType === 9 ? this.documentElement : this.nodeType === 1 ? this : this.parentElement
  if (!element || !element.isConnected) {
    throw new Error('Accessibility target was removed; call getAXState() again')
  }
  globalThis.__jazRefMap ??= new Map()
  globalThis.__jazRefMap.set(ref, element)
  try {
    return eval(expression)
  } finally {
    globalThis.__jazRefMap.delete(ref)
  }
}`,
		"arguments":     []any{map[string]any{"value": ref}, map[string]any{"value": expression}},
		"returnByValue": true,
	}, &result); err != nil {
		return err
	}
	if err := result.decode(out); err != nil {
		return err
	}
	if point, ok := out.(*pointResult); ok && point.Found {
		return p.accessibilityPoint(ctx, target, point, frames)
	}
	return nil
}

func (p *browserPage) accessibilityPoint(ctx context.Context, target axTarget, point *pointResult, frames []axFrame) error {
	byID := map[string]axFrame{}
	for _, frame := range frames {
		byID[frame.ID] = frame
	}
	frame := target.Frame
	if parent, ok := byID[frame.ParentID]; ok && frame.SessionID == parent.SessionID {
		if err := p.framePoint(ctx, frame, parent, point); err != nil {
			return err
		}
		if err := p.hitFrame(ctx, frame.SessionID, frame.ID, 0, point); err != nil {
			return err
		}
	}
	for frame.ParentID != "" {
		parent := byID[frame.ParentID]
		if frame.SessionID != parent.SessionID {
			if err := p.framePoint(ctx, frame, parent, point); err != nil {
				return err
			}
			if err := p.hitFrame(ctx, parent.SessionID, parent.ID, frame.OwnerID, point); err != nil {
				return err
			}
		}
		frame = parent
	}
	return nil
}

func (p *browserPage) framePoint(ctx context.Context, frame, parent axFrame, point *pointResult) error {
	owner, err := p.contentQuad(ctx, parent.SessionID, frame.OwnerID)
	if err != nil {
		return err
	}
	var viewport struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	var world struct {
		ID int64 `json:"executionContextId"`
	}
	if err := p.conn.callSession(ctx, frame.SessionID, "Page.createIsolatedWorld", map[string]any{"frameId": frame.ID, "worldName": "jaz-browser-control"}, &world); err != nil {
		return err
	}
	if err := p.evalInSession(ctx, frame.SessionID, world.ID, `({width: innerWidth, height: innerHeight})`, &viewport); err != nil {
		return err
	}
	if viewport.Width <= 0 || viewport.Height <= 0 {
		return fmt.Errorf("accessibility frame %s is not visible", frame.ID)
	}
	x, y := point.X/viewport.Width, point.Y/viewport.Height
	point.X = owner[0] + x*(owner[2]-owner[0]) + y*(owner[6]-owner[0])
	point.Y = owner[1] + x*(owner[3]-owner[1]) + y*(owner[7]-owner[1])
	return nil
}

func (p *browserPage) hitFrame(ctx context.Context, sessionID, frameID string, ownerID int64, point *pointResult) error {
	var hit struct {
		FrameID   string `json:"frameId"`
		BackendID int64  `json:"backendNodeId"`
	}
	if err := p.conn.callSession(ctx, sessionID, "DOM.getNodeForLocation", map[string]any{
		"x":                         int(point.X),
		"y":                         int(point.Y),
		"includeUserAgentShadowDOM": true,
	}, &hit); err != nil {
		return err
	}
	if hit.FrameID != frameID || (ownerID != 0 && hit.BackendID != ownerID) {
		return fmt.Errorf("%s is obscured by another frame or element; read the page again", point.Label)
	}
	return nil
}

func (p *browserPage) contentQuad(ctx context.Context, sessionID string, backendID int64) ([]float64, error) {
	var result struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err := p.conn.callSession(ctx, sessionID, "DOM.getBoxModel", map[string]any{"backendNodeId": backendID}, &result); err != nil {
		return nil, err
	}
	if len(result.Model.Content) != 8 {
		return nil, fmt.Errorf("accessibility target has no visible layout; read its state again")
	}
	return result.Model.Content, nil
}
