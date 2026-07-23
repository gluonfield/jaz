package browsercontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (p *browserPage) navigate(ctx context.Context, rawURL string) (ActionOutput, error) {
	target, err := normalizeBrowserURL(rawURL)
	if err != nil {
		return ActionOutput{}, err
	}
	var navigation struct {
		LoaderID  string `json:"loaderId"`
		ErrorText string `json:"errorText"`
	}
	if err := p.conn.call(ctx, "Page.navigate", map[string]any{"url": target}, &navigation); err != nil {
		return ActionOutput{}, err
	}
	if strings.TrimSpace(navigation.ErrorText) != "" {
		return ActionOutput{}, fmt.Errorf("browser navigation failed: %s", strings.TrimSpace(navigation.ErrorText))
	}
	if navigation.LoaderID != "" {
		p.contextID = 0
	}
	if err := p.waitNavigationCommit(ctx, target, navigation.LoaderID); err != nil {
		return ActionOutput{}, err
	}
	if err := p.waitReady(ctx); err != nil {
		return ActionOutput{}, fmt.Errorf("browser navigation did not become ready: %w", err)
	}
	info, err := p.info(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Navigated.\n" + info}, nil
}

func (p *browserPage) semanticState(ctx context.Context) (ActionOutput, error) {
	var data json.RawMessage
	if err := p.eval(ctx, semanticStateScript(), &data); err != nil {
		return ActionOutput{}, err
	}
	state, ok := decodePageState(data)
	text := strings.TrimSpace(string(data))
	if ok {
		text = formatPageState(state)
	}
	return ActionOutput{Status: "ok", Text: text, Data: data}, nil
}

func (p *browserPage) find(ctx context.Context, query string) (ActionOutput, error) {
	var data json.RawMessage
	if err := p.eval(ctx, findElementsScript(query), &data); err != nil {
		return ActionOutput{}, err
	}
	state, ok := decodePageState(data)
	text := strings.TrimSpace(string(data))
	if ok {
		text = formatPageState(state)
	}
	return ActionOutput{Status: "ok", Text: text, Data: data}, nil
}

func (p *browserPage) screenshot(ctx context.Context) (ActionOutput, error) {
	var out struct {
		Data string `json:"data"`
	}
	if err := p.conn.call(ctx, "Page.captureScreenshot", map[string]any{
		"format":      "png",
		"fromSurface": true,
	}, &out); err != nil {
		return ActionOutput{}, err
	}
	data, err := decodeImageBase64(out.Data)
	if err != nil {
		return ActionOutput{}, err
	}
	info, _ := p.info(ctx)
	return ActionOutput{
		Status:        "ok",
		Text:          "Screenshot captured.\n" + info,
		ImageData:     data,
		ImageMIMEType: "image/png",
	}, nil
}

func (p *browserPage) click(ctx context.Context, ref string) (ActionOutput, error) {
	point, err := p.resolvePoint(ctx, ref)
	if err != nil {
		return ActionOutput{}, err
	}
	if err := p.mouse(ctx, "mouseMoved", point.X, point.Y); err != nil {
		return ActionOutput{}, err
	}
	if err := p.mouse(ctx, "mousePressed", point.X, point.Y, "button", "left", "clickCount", 1); err != nil {
		return ActionOutput{}, err
	}
	if err := p.mouse(ctx, "mouseReleased", point.X, point.Y, "button", "left", "clickCount", 1); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Clicked " + point.Label + "."}, nil
}

func (p *browserPage) formInput(ctx context.Context, ref string, value any) (ActionOutput, error) {
	var out elementResult
	if err := p.eval(ctx, formInputScript(ref, value), &out); err != nil {
		return ActionOutput{}, err
	}
	if !out.Found {
		return ActionOutput{}, fmt.Errorf("element ref was not found or is stale: %s", strings.TrimPrefix(ref, "ref="))
	}
	if !out.Changed {
		if out.Reason != "" {
			return ActionOutput{}, errors.New(out.Reason)
		}
		return ActionOutput{}, fmt.Errorf("value is not valid for %s", out.Label)
	}
	return ActionOutput{Status: "ok", Text: "Set " + out.Label + "."}, nil
}

func (p *browserPage) press(ctx context.Context, ref, key string) (ActionOutput, error) {
	if ref != "" {
		if err := p.focus(ctx, ref); err != nil {
			return ActionOutput{}, err
		}
	}
	event, err := keyEvent(strings.TrimSpace(key))
	if err != nil {
		return ActionOutput{}, err
	}
	down := copyMap(event)
	down["type"] = "keyDown"
	up := copyMap(event)
	up["type"] = "keyUp"
	if err := p.conn.call(ctx, "Input.dispatchKeyEvent", down, nil); err != nil {
		return ActionOutput{}, err
	}
	if err := p.conn.call(ctx, "Input.dispatchKeyEvent", up, nil); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Pressed " + event["key"].(string) + "."}, nil
}

func (p *browserPage) scroll(ctx context.Context, ref, direction string, amount int) (ActionOutput, error) {
	x, y := 0.0, 0.0
	if strings.TrimSpace(ref) != "" {
		point, err := p.resolvePoint(ctx, ref)
		if err != nil {
			return ActionOutput{}, err
		}
		x, y = point.X, point.Y
	} else {
		var viewport struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		}
		if err := p.eval(ctx, `({x: innerWidth / 2, y: innerHeight / 2})`, &viewport); err != nil {
			return ActionOutput{}, err
		}
		x, y = viewport.X, viewport.Y
	}
	deltaY, deltaX := scrollDelta(direction, amount)
	params := map[string]any{
		"type":   "mouseWheel",
		"x":      x,
		"y":      y,
		"deltaX": deltaX,
		"deltaY": deltaY,
	}
	if err := p.conn.call(ctx, "Input.dispatchMouseEvent", params, nil); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: fmt.Sprintf("Scrolled deltaX=%d deltaY=%d.", deltaX, deltaY)}, nil
}

func (p *browserPage) wait(ctx context.Context, ref, text string, timeoutMS int) (ActionOutput, error) {
	timeout := waitTimeout(timeoutMS)
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ref = strings.TrimSpace(ref)
	text = strings.TrimSpace(text)
	if ref == "" && text == "" {
		if err := p.waitReady(waitCtx); err != nil {
			return ActionOutput{}, fmt.Errorf("wait for page readiness: %w", err)
		}
		return ActionOutput{Status: "ok", Text: "Page is ready."}, nil
	}
	if ref != "" {
		if _, err := p.probeRef(waitCtx, ref); err != nil {
			return ActionOutput{}, fmt.Errorf("element ref is stale or unknown; call browser_read_page or browser_find again: %w", err)
		}
	}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, label, err := p.waitProbe(waitCtx, ref, text)
		if err != nil {
			return ActionOutput{}, fmt.Errorf("browser wait failed: %w", err)
		}
		if ok {
			return ActionOutput{Status: "ok", Text: "Wait condition satisfied: " + label + "."}, nil
		}
		select {
		case <-waitCtx.Done():
			return ActionOutput{}, fmt.Errorf("timed out waiting for %s after %s", waitLabel(ref, text), timeout)
		case <-ticker.C:
		}
	}
}

func (p *browserPage) waitProbe(ctx context.Context, ref, text string) (bool, string, error) {
	label := waitLabel(ref, text)
	if ref != "" {
		element, err := p.probeRef(ctx, ref)
		if err != nil {
			return false, label, err
		}
		label = element.Label
	}
	if text != "" {
		var found bool
		if err := p.eval(ctx, textPresentScript(text), &found); err != nil {
			return false, label, err
		}
		if !found {
			return false, label, nil
		}
	}
	return true, label, nil
}

func (p *browserPage) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var ready string
		err := p.eval(ctx, "document.readyState", &ready)
		if err == nil && (ready == "complete" || ready == "interactive") {
			return nil
		}
		if err != nil && !missingExecutionContext(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *browserPage) eval(ctx context.Context, expression string, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		contextID, err := p.isolatedContext(ctx)
		if err != nil {
			return err
		}
		err = p.evalInContext(ctx, contextID, expression, out)
		if err == nil {
			return nil
		}
		if attempt == 0 && missingExecutionContext(err) {
			p.contextID = 0
			continue
		}
		return err
	}
	return errors.New("browser execution context is unavailable")
}

func (p *browserPage) evalInContext(ctx context.Context, contextID int64, expression string, out any) error {
	var result struct {
		Result struct {
			Type        string          `json:"type"`
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	err := p.conn.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
		"contextId":     contextID,
	}, &result)
	if err != nil {
		return err
	}
	if len(result.ExceptionDetails) > 0 && !bytes.Equal(result.ExceptionDetails, []byte("null")) {
		return fmt.Errorf("browser JavaScript failed: %s", string(result.ExceptionDetails))
	}
	if out == nil {
		return nil
	}
	if len(result.Result.Value) == 0 {
		return json.Unmarshal([]byte("null"), out)
	}
	return json.Unmarshal(result.Result.Value, out)
}

func (p *browserPage) isolatedContext(ctx context.Context) (int64, error) {
	if p.contextID > 0 {
		return p.contextID, nil
	}
	frame, err := p.topFrame(ctx)
	if err != nil {
		return 0, err
	}
	var out struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := p.conn.call(ctx, "Page.createIsolatedWorld", map[string]any{
		"frameId":   frame.ID,
		"worldName": "jaz-browser-control",
	}, &out); err != nil {
		return 0, err
	}
	if out.ExecutionContextID <= 0 {
		return 0, errors.New("browser did not create an isolated execution context")
	}
	p.contextID = out.ExecutionContextID
	return p.contextID, nil
}

func (p *browserPage) waitNavigationCommit(ctx context.Context, targetURL, loaderID string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		frame, err := p.topFrame(waitCtx)
		if err != nil {
			return fmt.Errorf("inspect browser navigation: %w", err)
		}
		if loaderID != "" {
			if frame.LoaderID == loaderID {
				return nil
			}
		} else if sameBrowserURL(frame.URL, targetURL) {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for browser navigation commit: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (p *browserPage) topFrame(ctx context.Context) (cdpFrame, error) {
	var out struct {
		FrameTree struct {
			Frame cdpFrame `json:"frame"`
		} `json:"frameTree"`
	}
	if err := p.conn.call(ctx, "Page.getFrameTree", map[string]any{}, &out); err != nil {
		return cdpFrame{}, err
	}
	if strings.TrimSpace(out.FrameTree.Frame.ID) == "" {
		return cdpFrame{}, errors.New("browser did not report a top-level frame")
	}
	return out.FrameTree.Frame, nil
}

func missingExecutionContext(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "context") &&
		(strings.Contains(message, "destroy") || strings.Contains(message, "cannot find") || strings.Contains(message, "not found"))
}

func sameBrowserURL(left, right string) bool {
	leftURL, leftErr := normalizeBrowserURL(left)
	rightURL, rightErr := normalizeBrowserURL(right)
	return leftErr == nil && rightErr == nil && leftURL == rightURL
}

func (p *browserPage) resolvePoint(ctx context.Context, ref string) (pointResult, error) {
	var out pointResult
	if err := p.eval(ctx, resolvePointScript(ref), &out); err != nil {
		return pointResult{}, err
	}
	if !out.Found {
		if strings.TrimSpace(out.Reason) != "" {
			return pointResult{}, errors.New(out.Reason)
		}
		return pointResult{}, fmt.Errorf("element ref %q is stale or unknown; call browser_read_page or browser_find again", strings.TrimSpace(ref))
	}
	if out.Label == "" {
		out.Label = "element"
	}
	return out, nil
}

func (p *browserPage) focus(ctx context.Context, ref string) error {
	var out elementResult
	if err := p.eval(ctx, focusScript(ref), &out); err != nil {
		return err
	}
	if !out.Found {
		return fmt.Errorf("element ref %q is stale or unknown; call browser_read_page or browser_find again", strings.TrimSpace(ref))
	}
	return nil
}

func (p *browserPage) probeRef(ctx context.Context, ref string) (elementResult, error) {
	var out elementResult
	if err := p.eval(ctx, probeRefScript(ref), &out); err != nil {
		return elementResult{}, err
	}
	if !out.Found {
		return elementResult{}, fmt.Errorf("element ref %q is stale or unknown", strings.TrimSpace(ref))
	}
	return out, nil
}

func (p *browserPage) mouse(ctx context.Context, eventType string, x, y float64, kv ...any) error {
	params := map[string]any{"type": eventType, "x": x, "y": y}
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if ok {
			params[key] = kv[i+1]
		}
	}
	return p.conn.call(ctx, "Input.dispatchMouseEvent", params, nil)
}

func waitTimeout(ms int) time.Duration {
	if ms <= 0 {
		return 10 * time.Second
	}
	timeout := time.Duration(ms) * time.Millisecond
	if timeout > time.Minute {
		return time.Minute
	}
	return timeout
}

func waitLabel(ref, text string) string {
	var parts []string
	if ref != "" {
		parts = append(parts, "ref "+ref)
	}
	if text != "" {
		parts = append(parts, "text "+text)
	}
	return strings.Join(parts, " and ")
}

type pointResult struct {
	Found  bool    `json:"found"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Label  string  `json:"label"`
	Reason string  `json:"reason"`
}

type elementResult struct {
	Found   bool   `json:"found"`
	Changed bool   `json:"changed"`
	Label   string `json:"label"`
	Reason  string `json:"reason"`
}

type cdpFrame struct {
	ID       string `json:"id"`
	LoaderID string `json:"loaderId"`
	URL      string `json:"url"`
}
