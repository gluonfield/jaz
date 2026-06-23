package browserworker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (p *browserPage) navigate(ctx context.Context, rawURL string) (ActionOutput, error) {
	target, err := normalizeBrowserURL(rawURL)
	if err != nil {
		return ActionOutput{}, err
	}
	if err := p.conn.call(ctx, "Page.navigate", map[string]any{"url": target}, nil); err != nil {
		return ActionOutput{}, err
	}
	_ = p.waitReady(ctx)
	info, err := p.info(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Navigated.\n" + info}, nil
}

func (p *browserPage) snapshot(ctx context.Context) (ActionOutput, error) {
	var tree struct {
		Nodes []axNode `json:"nodes"`
	}
	if err := p.conn.call(ctx, "Accessibility.getFullAXTree", map[string]any{}, &tree); err != nil {
		return ActionOutput{}, err
	}
	info, err := p.info(ctx)
	if err != nil {
		return ActionOutput{}, err
	}
	var b strings.Builder
	b.WriteString(info)
	b.WriteString("\n\nAccessibility snapshot:\n")
	count := 0
	for _, node := range tree.Nodes {
		line := node.snapshotLine()
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		count++
		if count >= 220 {
			b.WriteString("... snapshot truncated ...\n")
			break
		}
	}
	if count == 0 {
		b.WriteString("(empty)\n")
	}
	return ActionOutput{Status: "ok", Text: b.String()}, nil
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

func (p *browserPage) extract(ctx context.Context) (ActionOutput, error) {
	var data json.RawMessage
	if err := p.eval(ctx, extractPageScript(), &data); err != nil {
		return ActionOutput{}, err
	}
	extraction, ok := decodePageExtraction(data)
	text := strings.TrimSpace(string(data))
	if ok {
		text = formatPageExtraction(extraction)
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
	info, _ := p.info(ctx)
	return ActionOutput{
		Status:        "ok",
		Text:          "Screenshot captured.\n" + info,
		ImageBase64:   out.Data,
		ImageMIMEType: "image/png",
	}, nil
}

func (p *browserPage) click(ctx context.Context, selector string) (ActionOutput, error) {
	point, err := p.resolvePoint(ctx, selector)
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

func (p *browserPage) hover(ctx context.Context, selector string) (ActionOutput, error) {
	point, err := p.resolvePoint(ctx, selector)
	if err != nil {
		return ActionOutput{}, err
	}
	if err := p.mouse(ctx, "mouseMoved", point.X, point.Y); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Hovered " + point.Label + "."}, nil
}

func (p *browserPage) typeText(ctx context.Context, selector, text string) (ActionOutput, error) {
	if strings.TrimSpace(selector) != "" {
		if err := p.focus(ctx, selector); err != nil {
			return ActionOutput{}, err
		}
	}
	if text == "" {
		return ActionOutput{Status: "ok", Text: "No text to type."}, nil
	}
	if err := p.conn.call(ctx, "Input.insertText", map[string]any{"text": text}, nil); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{Status: "ok", Text: "Typed text."}, nil
}

func (p *browserPage) fill(ctx context.Context, selector, text string) (ActionOutput, error) {
	var out elementResult
	if err := p.eval(ctx, setValueScript(selector, text, false), &out); err != nil {
		return ActionOutput{}, err
	}
	if !out.Found {
		return ActionOutput{}, fmt.Errorf("element not found: %s", strings.TrimSpace(selector))
	}
	return ActionOutput{Status: "ok", Text: "Filled " + out.Label + "."}, nil
}

func (p *browserPage) selectOption(ctx context.Context, selector, text string) (ActionOutput, error) {
	var out elementResult
	if err := p.eval(ctx, setValueScript(selector, text, true), &out); err != nil {
		return ActionOutput{}, err
	}
	if !out.Found {
		return ActionOutput{}, fmt.Errorf("element not found: %s", strings.TrimSpace(selector))
	}
	if !out.Changed {
		return ActionOutput{}, fmt.Errorf("option not found for %s", out.Label)
	}
	return ActionOutput{Status: "ok", Text: "Selected option in " + out.Label + "."}, nil
}

func (p *browserPage) press(ctx context.Context, key string) (ActionOutput, error) {
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

func (p *browserPage) scroll(ctx context.Context, selector, direction string, amount int) (ActionOutput, error) {
	x, y := 0.0, 0.0
	if strings.TrimSpace(selector) != "" {
		point, err := p.resolvePoint(ctx, selector)
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

func (p *browserPage) pdf(ctx context.Context) (ActionOutput, error) {
	var out struct {
		Data string `json:"data"`
	}
	if err := p.conn.call(ctx, "Page.printToPDF", map[string]any{"printBackground": true}, &out); err != nil {
		return ActionOutput{}, err
	}
	return ActionOutput{
		Status:          "ok",
		Text:            fmt.Sprintf("PDF captured (%d base64 characters).", len(out.Data)),
		PDFBase64:       out.Data,
		PDFBase64Length: len(out.Data),
	}, nil
}

func (p *browserPage) wait(ctx context.Context, selector, text string, timeoutMS int) (ActionOutput, error) {
	timeout := waitTimeout(timeoutMS)
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	selector = strings.TrimSpace(selector)
	text = strings.TrimSpace(text)
	if selector == "" && text == "" {
		if err := p.waitReady(waitCtx); err != nil {
			return ActionOutput{}, fmt.Errorf("timed out waiting for page readiness after %s", timeout)
		}
		return ActionOutput{Status: "ok", Text: "Page is ready."}, nil
	}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, label := p.waitProbe(waitCtx, selector, text)
		if ok {
			return ActionOutput{Status: "ok", Text: "Wait condition satisfied: " + label + "."}, nil
		}
		select {
		case <-waitCtx.Done():
			return ActionOutput{}, fmt.Errorf("timed out waiting for %s after %s", waitLabel(selector, text), timeout)
		case <-ticker.C:
		}
	}
}

func (p *browserPage) waitProbe(ctx context.Context, selector, text string) (bool, string) {
	label := waitLabel(selector, text)
	if selector != "" {
		point, err := p.resolvePoint(ctx, selector)
		if err != nil {
			return false, label
		}
		label = point.Label
	}
	if text != "" {
		var found bool
		if err := p.eval(ctx, textPresentScript(text), &found); err != nil || !found {
			return false, label
		}
	}
	return true, label
}

func (p *browserPage) waitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var ready string
		if err := p.eval(ctx, "document.readyState", &ready); err == nil && (ready == "complete" || ready == "interactive") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *browserPage) eval(ctx context.Context, expression string, out any) error {
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

func (p *browserPage) resolvePoint(ctx context.Context, selector string) (pointResult, error) {
	var out pointResult
	if err := p.eval(ctx, resolvePointScript(selector), &out); err != nil {
		return pointResult{}, err
	}
	if !out.Found {
		return pointResult{}, fmt.Errorf("element not found: %s", strings.TrimSpace(selector))
	}
	if out.Label == "" {
		out.Label = "element"
	}
	return out, nil
}

func (p *browserPage) focus(ctx context.Context, selector string) error {
	var out elementResult
	if err := p.eval(ctx, focusScript(selector), &out); err != nil {
		return err
	}
	if !out.Found {
		return fmt.Errorf("element not found: %s", strings.TrimSpace(selector))
	}
	return nil
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

func waitLabel(selector, text string) string {
	var parts []string
	if selector != "" {
		parts = append(parts, "selector "+selector)
	}
	if text != "" {
		parts = append(parts, "text "+text)
	}
	return strings.Join(parts, " and ")
}

type pointResult struct {
	Found bool    `json:"found"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label"`
}

type elementResult struct {
	Found   bool   `json:"found"`
	Changed bool   `json:"changed"`
	Label   string `json:"label"`
}
