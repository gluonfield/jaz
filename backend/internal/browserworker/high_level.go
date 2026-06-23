package browserworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const (
	ToolDo    = "browser_do"
	ToolGet   = "browser_get"
	ToolCheck = "browser_check"
)

type HighLevelInput struct {
	Task     string `json:"task,omitempty" jsonschema:"goal or question for this browser operation"`
	URL      string `json:"url,omitempty" jsonschema:"optional page URL to open first"`
	Action   string `json:"action,omitempty" jsonschema:"optional concrete action: navigate, click, type, fill, select, press, scroll, wait"`
	Selector string `json:"selector,omitempty" jsonschema:"selector, text locator, or ref=e1 target returned by browser_get"`
	Text     string `json:"text,omitempty" jsonschema:"text to type, fill, select, wait for, or check"`
	Key      string `json:"key,omitempty" jsonschema:"keyboard key for press actions"`
	Amount   int    `json:"amount,omitempty" jsonschema:"scroll amount in CSS pixels or wait timeout in milliseconds"`
	Visual   bool   `json:"visual,omitempty" jsonschema:"attach a screenshot when visual state matters"`
}

type highLevelKind string

const (
	highLevelDo    highLevelKind = "do"
	highLevelGet   highLevelKind = "get"
	highLevelCheck highLevelKind = "check"
)

type HighLevelExecutor struct {
	backend Backend
	mu      sync.Mutex
	states  map[string]PageState
}

func NewHighLevelExecutor(backend Backend) *HighLevelExecutor {
	if backend == nil {
		backend = UnavailableBackend{}
	}
	return &HighLevelExecutor{backend: backend, states: map[string]PageState{}}
}

func AddHighLevelMCPTools(server *mcp.Server, backend Backend) {
	tools := highLevelTools{executor: NewHighLevelExecutor(backend)}
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolDo,
		Title:       "Do browser action",
		Description: "Run a deterministic browser action through Jaz page refs and waits. Use low-level browser only for escape hatches.",
	}, tools.Do)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolGet,
		Title:       "Get browser state",
		Description: "Return compact page state with stable ref= targets. Set visual=true only when screenshot evidence is needed.",
	}, tools.Get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolCheck,
		Title:       "Check browser state",
		Description: "Check selector or text presence deterministically, returning compact evidence.",
	}, tools.Check)
}

func RemoveHighLevelMCPTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(ToolDo, ToolGet, ToolCheck)
	}
}

type highLevelTools struct {
	executor *HighLevelExecutor
}

func (t highLevelTools) Do(ctx context.Context, req *mcp.CallToolRequest, input HighLevelInput) (*mcp.CallToolResult, ActionOutput, error) {
	return t.run(ctx, req, highLevelDo, input)
}

func (t highLevelTools) Get(ctx context.Context, req *mcp.CallToolRequest, input HighLevelInput) (*mcp.CallToolResult, ActionOutput, error) {
	return t.run(ctx, req, highLevelGet, input)
}

func (t highLevelTools) Check(ctx context.Context, req *mcp.CallToolRequest, input HighLevelInput) (*mcp.CallToolResult, ActionOutput, error) {
	return t.run(ctx, req, highLevelCheck, input)
}

func (t highLevelTools) run(ctx context.Context, req *mcp.CallToolRequest, kind highLevelKind, input HighLevelInput) (*mcp.CallToolResult, ActionOutput, error) {
	session := mcpsession.SessionID(req)
	out, err := t.executor.Run(ctx, session, kind, input)
	if err != nil {
		return nil, ActionOutput{}, err
	}
	return contentResult(out), out, nil
}

func (e *HighLevelExecutor) Run(ctx context.Context, session string, kind highLevelKind, input HighLevelInput) (ActionOutput, error) {
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	if input.URL != "" && !(kind == highLevelDo && input.Action == "navigate") {
		if _, err := e.backend.Call(ctx, ActionInput{Action: "navigate", URL: input.URL, Session: session}); err != nil {
			return ActionOutput{}, err
		}
	}
	switch kind {
	case highLevelGet:
		return e.getState(ctx, session, input)
	case highLevelCheck:
		return e.checkState(ctx, session, input)
	default:
		return e.doAction(ctx, session, input)
	}
}

func (e *HighLevelExecutor) getState(ctx context.Context, session string, input HighLevelInput) (ActionOutput, error) {
	out, _, _, err := e.state(ctx, session)
	if err != nil {
		return ActionOutput{}, err
	}
	return e.attachVisual(ctx, session, input, out)
}

func (e *HighLevelExecutor) checkState(ctx context.Context, session string, input HighLevelInput) (ActionOutput, error) {
	var checks []string
	var ok = true
	selector := strings.TrimSpace(input.Selector)
	text := strings.TrimSpace(input.Text)
	if text == "" {
		text = quotedText(input.Task)
	}
	if selector != "" {
		_, err := e.backend.Call(ctx, ActionInput{Action: "wait", Selector: selector, Amount: shortWait(input.Amount), Session: session})
		if err != nil {
			ok = false
			checks = append(checks, "selector missing: "+selector)
		} else {
			checks = append(checks, "selector present: "+selector)
		}
	}
	stateOut, state, hasState, err := e.state(ctx, session)
	if err != nil {
		return ActionOutput{}, err
	}
	if text != "" {
		found := hasState && strings.Contains(normalizedText(state.Text), normalizedText(text))
		ok = ok && found
		if found {
			checks = append(checks, "text present: "+text)
		} else {
			checks = append(checks, "text missing: "+text)
		}
	}
	if len(checks) == 0 {
		checks = append(checks, "no exact selector or quoted text was supplied; use the compact state below as evidence")
	}
	stateOut.Text = fmt.Sprintf("Check: %t\n%s\n\n%s", ok, strings.Join(checks, "\n"), stateOut.Text)
	return e.attachVisual(ctx, session, input, stateOut)
}

func (e *HighLevelExecutor) doAction(ctx context.Context, session string, input HighLevelInput) (ActionOutput, error) {
	action := input.Action
	if action == "" {
		action = inferHighLevelAction(input)
	}
	if action == "" {
		return e.getState(ctx, session, input)
	}
	if action == "navigate" {
		out, err := e.backend.Call(ctx, ActionInput{Action: "navigate", URL: input.URL, Session: session})
		if err != nil {
			return ActionOutput{}, err
		}
		return e.attachStateAfterAction(ctx, session, input, out)
	}
	selector := strings.TrimSpace(input.Selector)
	if selector == "" && actionNeedsTarget(action) {
		_, state, hasState, err := e.state(ctx, session)
		if err != nil {
			return ActionOutput{}, err
		}
		if hasState {
			selector = bestElementSelector(state, input.Task)
		}
		if selector == "" {
			return ActionOutput{}, errors.New("could not identify a browser target; call browser_get or pass selector/ref")
		}
	}
	out, err := e.backend.Call(ctx, ActionInput{
		Action:   action,
		Selector: selector,
		Text:     input.Text,
		Key:      input.Key,
		Amount:   input.Amount,
		Session:  session,
	})
	if err != nil {
		return ActionOutput{}, err
	}
	return e.attachStateAfterAction(ctx, session, input, out)
}

func (e *HighLevelExecutor) attachStateAfterAction(ctx context.Context, session string, input HighLevelInput, actionOut ActionOutput) (ActionOutput, error) {
	_, _ = e.backend.Call(ctx, ActionInput{Action: "wait", Amount: shortWait(input.Amount), Session: session})
	stateOut, _, _, err := e.state(ctx, session)
	if err == nil && strings.TrimSpace(stateOut.Text) != "" {
		actionOut.Text = strings.TrimSpace(actionOut.Text + "\n\n" + stateOut.Text)
		actionOut.Data = stateOut.Data
	}
	return e.attachVisual(ctx, session, input, actionOut)
}

func (e *HighLevelExecutor) state(ctx context.Context, session string) (ActionOutput, PageState, bool, error) {
	out, err := e.backend.Call(ctx, ActionInput{Action: "state", Session: session})
	if err != nil {
		return ActionOutput{}, PageState{}, false, err
	}
	state, ok := decodePageState(out.Data)
	if !ok {
		return out, PageState{}, false, nil
	}
	out.Text = e.stateTextWithDiff(session, state)
	return out, state, true, nil
}

func (e *HighLevelExecutor) stateTextWithDiff(session string, state PageState) string {
	text := formatPageState(state)
	e.mu.Lock()
	previous, ok := e.states[session]
	e.states[session] = state
	e.mu.Unlock()
	if ok {
		if diff := pageStateDiff(previous, state); diff != "" {
			text = diff + "\n\n" + text
		}
	}
	return text
}

func pageStateDiff(previous, current PageState) string {
	var parts []string
	if previous.URL != "" && current.URL != "" && previous.URL != current.URL {
		parts = append(parts, "URL changed")
	}
	if previous.Title != "" && current.Title != "" && previous.Title != current.Title {
		parts = append(parts, "title changed")
	}
	if len(previous.Elements) != 0 && len(current.Elements) != 0 && len(previous.Elements) != len(current.Elements) {
		parts = append(parts, fmt.Sprintf("targets %d -> %d", len(previous.Elements), len(current.Elements)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "State diff: " + strings.Join(parts, ", ")
}

func (e *HighLevelExecutor) attachVisual(ctx context.Context, session string, input HighLevelInput, out ActionOutput) (ActionOutput, error) {
	if !wantsVisual(input) {
		return out, nil
	}
	shot, err := e.backend.Call(ctx, ActionInput{Action: "screenshot", Session: session})
	if err != nil {
		out.Text = strings.TrimSpace(out.Text + "\n\nScreenshot unavailable: " + err.Error())
		return out, nil
	}
	out.ImageBase64 = shot.ImageBase64
	out.ImageMIMEType = shot.ImageMIMEType
	return out, nil
}

func wantsVisual(input HighLevelInput) bool {
	if input.Visual {
		return true
	}
	task := normalizedText(input.Task)
	return strings.Contains(task, "screenshot") || strings.Contains(task, "image") || strings.Contains(task, "visual")
}

func inferHighLevelAction(input HighLevelInput) string {
	task := normalizedText(input.Task)
	switch {
	case input.URL != "" && task == "":
		return "navigate"
	case strings.Contains(task, "wait"):
		return "wait"
	case strings.Contains(task, "scroll"):
		return "scroll"
	case input.Key != "" || strings.Contains(task, "press"):
		return "press"
	case input.Text != "" && strings.Contains(task, "select"):
		return "select"
	case input.Text != "" && (strings.Contains(task, "fill") || strings.Contains(task, "type") || strings.Contains(task, "enter") || strings.Contains(task, "write") || strings.Contains(task, "search")):
		return "fill"
	case input.Selector != "" || task != "":
		return "click"
	default:
		return ""
	}
}

func actionNeedsTarget(action string) bool {
	switch action {
	case "click", "hover", "type", "fill", "select":
		return true
	default:
		return false
	}
}

func bestElementSelector(state PageState, query string) string {
	tokens := significantTokens(query)
	if len(tokens) == 0 {
		return ""
	}
	bestRef := ""
	bestScore := 0
	for _, element := range state.Elements {
		haystack := normalizedText(strings.Join([]string{element.Role, element.Name, element.Text, element.Href}, " "))
		score := 0
		for _, token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if haystack == normalizedText(query) {
			score += 4
		}
		if score > bestScore {
			bestScore = score
			bestRef = strings.TrimSpace(element.Ref)
		}
	}
	if bestScore == 0 || bestRef == "" {
		return ""
	}
	return "ref=" + bestRef
}

func significantTokens(value string) []string {
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "button": true, "click": true, "for": true, "in": true, "into": true,
		"link": true, "on": true, "open": true, "press": true, "select": true, "the": true, "to": true,
	}
	var out []string
	for _, raw := range strings.FieldsFunc(normalizedText(value), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if len(raw) >= 2 && !stop[raw] {
			out = append(out, raw)
		}
	}
	return out
}

func quotedText(value string) string {
	for _, quote := range []string{`"`, `'`} {
		parts := strings.Split(value, quote)
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func normalizedText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func shortWait(amount int) int {
	if amount > 0 && amount < 3000 {
		return amount
	}
	return 3000
}
