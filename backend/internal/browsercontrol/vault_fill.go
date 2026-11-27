package browsercontrol

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type VaultFillField struct {
	Slot  string
	Ref   string
	Value string
}

type VaultFillRequest struct {
	ExpectedOrigin      string
	AllowPaymentIframes bool
	Fields              []VaultFillField
}

type VaultFillResult struct {
	FilledSlots []string
}

var allowlistedPaymentHosts = map[string]bool{
	"js.stripe.com":                  true,
	"checkout.stripe.com":            true,
	"elements.stripe.com":            true,
	"assets.braintreegateway.com":    true,
	"elements.braintreegateway.com":  true,
	"checkoutshopper-live.adyen.com": true,
	"checkoutshopper-test.adyen.com": true,
	"pci-connect.squareup.com":       true,
	"www.paypal.com":                 true,
}

func (b *DesktopBackend) ActivePageOrigin(ctx context.Context, sessionID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	page, err := b.page(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !page.opMu.TryLock() {
		return "", errors.New("Another browser action is running for this conversation")
	}
	defer page.opMu.Unlock()
	frame, err := page.topFrame(ctx)
	if err != nil {
		return "", err
	}
	return frameOrigin(frame.URL)
}

func (b *DesktopBackend) BlindFill(ctx context.Context, sessionID string, req VaultFillRequest) (VaultFillResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	page, err := b.page(ctx, sessionID)
	if err != nil {
		return VaultFillResult{}, err
	}
	if !page.opMu.TryLock() {
		return VaultFillResult{}, errors.New("Another browser action is running for this conversation")
	}
	defer page.opMu.Unlock()
	return page.blindFill(ctx, req)
}

func (p *browserPage) blindFill(ctx context.Context, req VaultFillRequest) (VaultFillResult, error) {
	expectedOrigin, err := frameOrigin(req.ExpectedOrigin)
	if err != nil {
		return VaultFillResult{}, fmt.Errorf("invalid expected origin: %w", err)
	}
	top, err := p.topFrame(ctx)
	if err != nil {
		return VaultFillResult{}, err
	}
	liveOrigin, err := frameOrigin(top.URL)
	if err != nil || liveOrigin != expectedOrigin {
		return VaultFillResult{}, fmt.Errorf("active tab origin %q does not match expected origin %q", top.URL, expectedOrigin)
	}
	filled := make([]string, 0, len(req.Fields))
	for _, field := range req.Fields {
		ref := strings.TrimSpace(field.Ref)
		if ref == "" {
			return VaultFillResult{}, fmt.Errorf("missing ref for slot %q", field.Slot)
		}
		if strings.HasPrefix(ref, "ax:") {
			target, ok := p.ax.targets[ref]
			if !ok {
				return VaultFillResult{}, fmt.Errorf("accessibility index %s is unavailable; call browser_get_ax_state again", ref)
			}
			targetOrigin, err := frameOrigin(target.Frame.URL)
			if err != nil {
				return VaultFillResult{}, fmt.Errorf("target frame for %s has unsupported origin", field.Slot)
			}
			if targetOrigin != expectedOrigin && (!req.AllowPaymentIframes || !isPaymentIframeOrigin(targetOrigin)) {
				return VaultFillResult{}, fmt.Errorf("target iframe origin %q is not authorized for slot %q", targetOrigin, field.Slot)
			}
			p.markRedactedNode(target.Frame.ID, target.BackendID)
		} else if backendID := p.describeTopRefBackendID(ctx, ref); backendID != 0 {
			p.markRedactedNode(top.ID, backendID)
		}
		var out elementResult
		if err := p.evalRef(ctx, ref, setControlValueScript(ref, field.Value, true), &out); err != nil {
			return VaultFillResult{}, fmt.Errorf("fill slot %q: %w", field.Slot, err)
		}
		if !out.Found {
			return VaultFillResult{}, fmt.Errorf("element ref for slot %q was not found or is stale", field.Slot)
		}
		if !out.Changed {
			reason := out.Reason
			if reason == "" {
				reason = "browser rejected the vault value"
			}
			return VaultFillResult{}, fmt.Errorf("fill slot %q failed: %s", field.Slot, reason)
		}
		filled = append(filled, field.Slot)
	}
	return VaultFillResult{FilledSlots: filled}, nil
}

func (p *browserPage) describeTopRefBackendID(ctx context.Context, ref string) int64 {
	if p.contextID == 0 {
		return 0
	}
	expr := elementResolverJS() + `(function(q){ return jazFindElement(q) || null; })(` + jsString(ref) + `);`
	var evalOut struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if err := p.conn.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expr,
		"contextId":     p.contextID,
		"returnByValue": false,
	}, &evalOut); err != nil || evalOut.Result.ObjectID == "" {
		return 0
	}
	defer func() {
		_ = p.conn.Call(ctx, "Runtime.releaseObject", map[string]any{"objectId": evalOut.Result.ObjectID}, nil)
	}()
	var described struct {
		Node struct {
			BackendNodeID int64 `json:"backendNodeId"`
		} `json:"node"`
	}
	if err := p.conn.Call(ctx, "DOM.describeNode", map[string]any{"objectId": evalOut.Result.ObjectID}, &described); err != nil {
		return 0
	}
	return described.Node.BackendNodeID
}

func (p *browserPage) markRedactedNode(frameID string, backendID int64) {
	if frameID == "" || backendID == 0 {
		return
	}
	if p.redactedNodes == nil {
		p.redactedNodes = map[string]map[int64]bool{}
	}
	if p.redactedNodes[frameID] == nil {
		p.redactedNodes[frameID] = map[int64]bool{}
	}
	p.redactedNodes[frameID][backendID] = true
}

func (p *browserPage) maskSensitiveForScreenshot(ctx context.Context) func() {
	maskCtx, cancelMask := context.WithTimeout(ctx, time.Second)
	defer cancelMask()
	type maskedWorld struct {
		sessionID string
		contextID int64
	}
	var worlds []maskedWorld
	frames, err := p.accessibilityFrames(maskCtx)
	if err == nil && len(frames) > 0 {
		for _, frame := range frames {
			var world struct {
				ID int64 `json:"executionContextId"`
			}
			if err := p.conn.CallSession(maskCtx, frame.SessionID, "Page.createIsolatedWorld", map[string]any{
				"frameId":   frame.ID,
				"worldName": "jaz-browser-control",
			}, &world); err == nil && world.ID > 0 {
				var count int
				if err := p.evalInSession(maskCtx, frame.SessionID, world.ID, screenshotMaskScript(true), &count); err == nil && count > 0 {
					worlds = append(worlds, maskedWorld{sessionID: frame.SessionID, contextID: world.ID})
				}
			}
		}
	} else {
		var count int
		if err := p.eval(maskCtx, screenshotMaskScript(true), &count); err == nil && count > 0 && p.contextID > 0 {
			worlds = append(worlds, maskedWorld{sessionID: "", contextID: p.contextID})
		}
	}
	return func() {
		if len(worlds) == 0 {
			return
		}
		restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, w := range worlds {
			_ = p.evalInSession(restoreCtx, w.sessionID, w.contextID, screenshotMaskScript(false), nil)
		}
	}
}

func screenshotMaskScript(enable bool) string {
	if enable {
		return elementResolverJS() + `
	(function(){
	  const list = globalThis.__jazMaskedNodes || [];
	  const seen = new WeakSet(list.map(item => item && item.el).filter(Boolean));
	  globalThis.__jazMaskedNodes = list;
	  let count = 0;
	  for (const el of jazAll("input,textarea,select")) {
	    if (seen.has(el) || !jazIsSensitiveControl(el)) continue;
	    list.push({
	      el,
	      filter: el.style.filter,
	      sec: el.style.webkitTextSecurity,
	      color: el.style.color,
	      caret: el.style.caretColor
	    });
	    el.style.webkitTextSecurity = "disc";
	    el.style.filter = "blur(6px)";
	    el.style.color = "transparent";
	    el.style.caretColor = "transparent";
	    count++;
	  }
	  return count;
	})()`
	}
	return `
	(function(){
	  const list = globalThis.__jazMaskedNodes || [];
	  for (let i = list.length - 1; i >= 0; i--) {
	    const item = list[i];
	    if (!item || !item.el) continue;
	    item.el.style.filter = item.filter || "";
	    item.el.style.webkitTextSecurity = item.sec || "";
	    item.el.style.color = item.color || "";
	    item.el.style.caretColor = item.caret || "";
	  }
	  globalThis.__jazMaskedNodes = [];
	  return true;
	})()`
}

func frameOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid frame URL %q", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	switch scheme {
	case "https":
	case "http":
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return "", fmt.Errorf("frame origin %q must use HTTPS or localhost", raw)
		}
	default:
		return "", fmt.Errorf("unsupported frame scheme %q", u.Scheme)
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return scheme + "://" + host + ":" + port, nil
	}
	return scheme + "://" + host, nil
}

func isPaymentIframeOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || strings.ToLower(u.Scheme) != "https" {
		return false
	}
	if port := u.Port(); port != "" && port != "443" {
		return false
	}
	return allowlistedPaymentHosts[strings.ToLower(u.Hostname())]
}
