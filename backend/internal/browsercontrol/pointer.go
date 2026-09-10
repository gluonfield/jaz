package browsercontrol

import (
	"context"
	"errors"
	"time"
)

func (p *browserPage) hover(ctx context.Context, ref string) (ActionOutput, error) {
	point, err := p.resolvePoint(ctx, ref)
	if err != nil {
		return ActionOutput{}, err
	}
	if err := p.mouse(ctx, "mouseMoved", point.X, point.Y); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Hovered " + point.Label + "."}, nil
}

func (p *browserPage) drag(ctx context.Context, from, to string) (ActionOutput, error) {
	start, err := p.resolvePoint(ctx, from)
	if err != nil {
		return ActionOutput{}, err
	}
	end, err := p.resolvePoint(ctx, to)
	if err != nil {
		return ActionOutput{}, err
	}
	// Resolving the destination may scroll; locate the source again without scrolling.
	if err := p.evalRef(ctx, from, pointScript(from, false), &start); err != nil {
		return ActionOutput{}, err
	}
	if !start.Found {
		return ActionOutput{}, errors.New("drag requires both elements visible at the same time; scroll and read the page again")
	}
	if err := p.mouse(ctx, "mouseMoved", start.X, start.Y); err != nil {
		return ActionOutput{}, err
	}
	if err := p.mouse(ctx, "mousePressed", start.X, start.Y, "button", "left", "buttons", 1, "clickCount", 1); err != nil {
		return ActionOutput{}, err
	}
	x, y := start.X, start.Y
	released := false
	defer func() {
		if released {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = p.mouse(releaseCtx, "mouseReleased", x, y, "button", "left", "clickCount", 1)
	}()
	for step := 1; step <= 12; step++ {
		progress := float64(step) / 12
		x = start.X + (end.X-start.X)*progress
		y = start.Y + (end.Y-start.Y)*progress
		if err := p.mouse(ctx, "mouseMoved", x, y, "button", "left", "buttons", 1); err != nil {
			return ActionOutput{}, err
		}
	}
	if err := p.mouse(ctx, "mouseReleased", x, y, "button", "left", "clickCount", 1); err != nil {
		return ActionOutput{}, err
	}
	released = true
	return ActionOutput{Status: "ok", Text: "Dragged " + start.Label + " to " + end.Label + "."}, nil
}
