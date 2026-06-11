package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wins/jaz/backend/internal/chat"
	"github.com/wins/jaz/backend/internal/provider"
)

type Config struct {
	Chat    chat.Client
	Session chat.Session
}

func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	history, err := cfg.Chat.LoadHistory(ctx, cfg.Session.ID)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(newChatModel(ctx, cancel, cfg, history)).Run()
	return err
}

type entry struct {
	agent   string
	role    string
	content string
}

type model struct {
	ctx          context.Context
	cancel       context.CancelFunc
	config       Config
	input        textinput.Model
	viewport     viewport.Model
	entries      []entry
	events       <-chan chat.Result
	stream       <-chan chat.Result
	streaming    bool
	pendingEsc   bool
	reply        int
	width        int
	contentWidth int
	height       int
	status       string
	skipFinalMsg bool
}

func newChatModel(ctx context.Context, cancel context.CancelFunc, cfg Config, history []chat.Event) model {
	input := textinput.New()
	input.Prompt = inputPrompt(cfg.Session)
	input.Placeholder = "send message"
	input.SetVirtualCursor(true)
	input.SetStyles(inputStyles())
	input.Focus()
	vp := viewport.New()
	vp.SoftWrap = true
	m := model{
		ctx:      ctx,
		cancel:   cancel,
		config:   cfg,
		input:    input,
		viewport: vp,
		entries:  entriesFromEvents(history, cfg.Session),
		events:   cfg.Chat.Subscribe(ctx, cfg.Session.ID),
		reply:    -1,
		status:   "ready",
	}
	m.resize(80, 24)
	m.syncViewport()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), waitAsync(m.events))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	dirty := false
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		dirty = true
	case tea.KeyPressMsg:
		escapeBackward := m.pendingEsc && isBackwardDelete(msg)
		escapeForward := m.pendingEsc && isForwardDelete(msg)
		m.pendingEsc = false
		if escapeBackward {
			if !m.streaming {
				m.deleteInputWordBackward()
			}
			return m, nil
		}
		if escapeForward {
			if !m.streaming {
				m.deleteInputWordForward()
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "esc":
			m.pendingEsc = true
			return m, nil
		case "enter":
			return m.submit()
		}
		if isDeleteWordForward(msg) {
			if !m.streaming {
				m.deleteInputWordForward()
			}
			return m, nil
		} else if isDeleteWordBackward(msg) {
			if !m.streaming {
				m.deleteInputWordBackward()
			}
			return m, nil
		}
	case streamResultMsg:
		m.applyStream(msg.result)
		dirty = true
		if m.streaming {
			cmds = append(cmds, waitStream(m.stream))
		}
	case asyncResultMsg:
		if msg.result.Err != nil {
			m.add(entry{role: "error", content: msg.result.Err.Error()})
			dirty = true
		} else if m.applyEvent(msg.result.Event) {
			dirty = true
		}
		cmds = append(cmds, waitAsync(m.events))
	case streamClosedMsg:
		m.streaming = false
		m.status = "ready"
	case asyncClosedMsg:
		m.status = "events disconnected"
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	if !m.streaming {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	if dirty {
		m.syncViewport()
	}
	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	header := renderHeader(m.config.Session, m.width)
	body := bodyStyle.Width(m.width).Height(max(m.height-5, 1)).Render(m.viewport.View())
	input := inputBoxStyle.Width(max(m.width-2, 20)).Render(m.input.View())
	status := renderStatus(m.status, m.config.Chat.ServerURL, m.width)
	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, input, status))
	view.AltScreen = true
	view.KeyboardEnhancements.ReportAlternateKeys = true
	view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	view.KeyboardEnhancements.ReportAssociatedText = true
	return view
}

func (m *model) resize(width, height int) {
	m.width = max(width, 40)
	m.contentWidth = max(m.width-2, 20)
	m.height = max(height, 8)
	m.input.SetWidth(max(m.contentWidth-lipgloss.Width(m.input.Prompt)-2, 10))
	m.viewport.SetWidth(m.contentWidth)
	m.viewport.SetHeight(max(m.height-5, 1))
}

func (m model) submit() (tea.Model, tea.Cmd) {
	message := strings.TrimSpace(m.input.Value())
	if message == "" {
		return m, nil
	}
	if message == "/quit" || message == "/exit" {
		m.cancel()
		return m, tea.Quit
	}
	if m.streaming {
		m.status = "waiting for current response"
		return m, nil
	}
	m.input.Reset()
	m.reply = -1
	m.streaming = true
	m.status = "streaming"
	m.stream = m.config.Chat.Stream(m.ctx, m.config.Session.ID, message)
	m.skipFinalMsg = false
	m.syncViewport()
	return m, waitStream(m.stream)
}

func (m *model) applyStream(result chat.Result) {
	if result.Err != nil {
		m.add(entry{role: "error", content: result.Err.Error()})
		m.streaming = false
		m.status = "ready"
		return
	}
	m.applyEvent(result.Event)
	switch result.Event.Type {
	case chat.EventError:
		m.streaming = false
		m.status = "ready"
		m.skipFinalMsg = false
	case chat.EventDone:
		m.streaming = false
		m.status = "ready"
		m.skipFinalMsg = false
	}
}

func (m *model) applyEvent(event chat.Event) bool {
	if event.Type == chat.EventAssistantDelta {
		if event.Delta != "" {
			m.appendReply(event.Delta)
			m.skipFinalMsg = true
		}
		return true
	}
	if event.Type == chat.EventAssistantMessage && m.skipFinalMsg {
		return true
	}
	entry, ok := entryFromEvent(event, m.config.Session)
	if !ok {
		return false
	}
	m.add(entry)
	return true
}

func (m *model) appendReply(delta string) {
	if m.reply < 0 || m.reply >= len(m.entries) {
		m.entries = append(m.entries, entry{role: "assistant", agent: sessionAgent(m.config.Session)})
		m.reply = len(m.entries) - 1
	}
	m.entries[m.reply].content += delta
}

func (m *model) add(entry entry) {
	if entry.content != "" {
		m.entries = append(m.entries, entry)
	}
}

func (m *model) deleteInputWordBackward() {
	value := []rune(m.input.Value())
	pos := min(m.input.Position(), len(value))
	if pos <= 0 || len(value) == 0 {
		return
	}
	i := pos
	for i > 0 && unicode.IsSpace(value[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(value[i-1]) {
		i--
	}
	next := append([]rune{}, value[:i]...)
	next = append(next, value[pos:]...)
	m.input.SetValue(string(next))
	m.input.SetCursor(i)
}

func (m *model) deleteInputWordForward() {
	value := []rune(m.input.Value())
	pos := min(m.input.Position(), len(value))
	if pos >= len(value) {
		return
	}
	i := pos
	for i < len(value) && unicode.IsSpace(value[i]) {
		i++
	}
	for i < len(value) && !unicode.IsSpace(value[i]) {
		i++
	}
	next := append([]rune{}, value[:pos]...)
	next = append(next, value[i:]...)
	m.input.SetValue(string(next))
	m.input.SetCursor(pos)
}

func isDeleteWordBackward(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	if key.Code == tea.KeyBackspace && hasWordDeleteMod(key.Mod) {
		return true
	}
	if unicode.ToLower(key.Code) == 'h' && key.Mod.Contains(tea.ModCtrl) && (key.Mod.Contains(tea.ModAlt) || key.Mod.Contains(tea.ModMeta)) {
		return true
	}
	switch msg.String() {
	case "alt+backspace", "meta+backspace", "ctrl+backspace", "ctrl+w", "ctrl+alt+h", "alt+ctrl+h", "ctrl+meta+h", "meta+ctrl+h":
		return true
	default:
		return false
	}
}

func isDeleteWordForward(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	if key.Code == tea.KeyDelete && hasWordDeleteMod(key.Mod) {
		return true
	}
	if unicode.ToLower(key.Code) == 'd' && (key.Mod.Contains(tea.ModAlt) || key.Mod.Contains(tea.ModMeta)) {
		return true
	}
	switch msg.String() {
	case "alt+delete", "meta+delete", "ctrl+delete", "alt+d", "meta+d":
		return true
	default:
		return false
	}
}

func hasWordDeleteMod(mod tea.KeyMod) bool {
	return mod.Contains(tea.ModAlt) || mod.Contains(tea.ModMeta) || mod.Contains(tea.ModCtrl)
}

func isBackwardDelete(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return key.Code == tea.KeyBackspace || unicode.ToLower(key.Code) == 'h' && key.Mod.Contains(tea.ModCtrl) || msg.String() == "backspace" || msg.String() == "ctrl+h"
}

func isForwardDelete(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return key.Code == tea.KeyDelete || unicode.ToLower(key.Code) == 'd' || msg.String() == "delete"
}

func (m *model) syncViewport() {
	m.viewport.SetContent(renderEntries(m.entries, m.contentWidth))
	m.viewport.GotoBottom()
}

func renderEntries(entries []entry, width int) string {
	var out []string
	var previous string
	for _, item := range entries {
		item.content = strings.TrimSpace(item.content)
		if item.content == "" {
			continue
		}
		if needsGap(previous, item.role) && len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, renderEntry(item, width)...)
		previous = item.role
	}
	return strings.Join(out, "\n")
}

func renderEntry(entry entry, width int) []string {
	if entry.role == "user" {
		return renderUserEntry(entry, width)
	}
	content := entry.content
	if isToolRole(entry.role) {
		content = "• " + content
	}
	body := entryStyle(entry).Width(max(width, 20)).Render(content)
	return strings.Split(strings.TrimRight(body, "\n"), "\n")
}

func renderUserEntry(entry entry, width int) []string {
	block := userMessageStyle.Width(max(width, 20)).Render(entry.content)
	return strings.Split(strings.TrimRight(block, "\n"), "\n")
}

func isToolRole(role string) bool {
	return role == "tool" || role == "tool result" || role == "activity" || role == "async"
}

func needsGap(previous, next string) bool {
	if previous == "" {
		return false
	}
	return isMessageRole(previous) || isMessageRole(next)
}

func isMessageRole(role string) bool {
	return role == "user" || role == "assistant" || role == "error"
}

func entryStyle(entry entry) lipgloss.Style {
	switch entry.role {
	case "tool", "tool result", "activity", "async":
		return toolTextStyle
	case "error":
		return errorTextStyle
	default:
		return messageTextStyle
	}
}

func entriesFromEvents(events []chat.Event, session chat.Session) []entry {
	entries := make([]entry, 0, len(events))
	for _, event := range events {
		if entry, ok := entryFromEvent(event, session); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func entryFromEvent(event chat.Event, session chat.Session) (entry, bool) {
	agent := sessionAgent(session)
	switch event.Type {
	case chat.EventUserMessage:
		return entry{role: "user", content: event.Content}, true
	case chat.EventAssistantMessage:
		return entry{role: "assistant", agent: agent, content: event.Content}, true
	case chat.EventToolCall:
		return entry{role: "tool", agent: agent, content: toolCallText(event)}, event.ToolName != "" || event.ToolCall != nil
	case chat.EventToolResult:
		return entry{role: "tool result", agent: agent, content: toolResultText(event.Result)}, true
	case chat.EventAsync:
		if event.ACP != nil {
			return entry{role: "async", agent: normalizeAgent(event.ACP.Agent), content: event.Content}, true
		}
		if agent != "" {
			return entry{role: "activity", agent: agent, content: event.Content}, true
		}
		return entry{role: "async", content: event.Content}, true
	case chat.EventError:
		return entry{role: "error", content: event.Error}, true
	default:
		return entry{}, false
	}
}

type streamResultMsg struct{ result chat.Result }
type streamClosedMsg struct{}

func waitStream(ch <-chan chat.Result) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return streamResultMsg{result: result}
	}
}

type asyncResultMsg struct{ result chat.Result }
type asyncClosedMsg struct{}

func waitAsync(ch <-chan chat.Result) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return asyncClosedMsg{}
		}
		return asyncResultMsg{result: result}
	}
}

func sessionLabel(session chat.Session) string {
	prefix := ""
	if agent := sessionAgent(session); agent != "" {
		prefix = agent + "/"
	}
	if session.Slug == "" || session.Slug == session.ID {
		return prefix + session.ID
	}
	return prefix + session.Slug
}

func inputPrompt(session chat.Session) string {
	return "› "
}

func inputStyles() textinput.Styles {
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = promptStyle
	styles.Blurred.Prompt = promptStyle
	styles.Focused.Text = messageTextStyle
	styles.Blurred.Text = messageTextStyle
	styles.Focused.Placeholder = mutedStyle
	styles.Blurred.Placeholder = mutedStyle
	return styles
}

func renderHeader(session chat.Session, width int) string {
	parts := []string{brandStyle.Render("Jaz"), sessionPillStyle.Render(sessionLabel(session))}
	if session.Slug != "" && session.Slug != session.ID {
		parts = append(parts, mutedStyle.Render(session.ID))
	}
	return headerStyle.Width(width).Render(strings.Join(parts, "  "))
}

func renderStatus(status, serverURL string, width int) string {
	left := statusReadyStyle.Render(status)
	if status != "ready" {
		left = statusBusyStyle.Render(status)
	}
	right := mutedStyle.Render(serverURL)
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right)-2, 1)
	return statusStyle.Width(width).Render(left + strings.Repeat(" ", gap) + right)
}

func compact(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func toolCallText(event chat.Event) string {
	name := event.ToolName
	args := ""
	if event.ToolCall != nil {
		name = provider.ToolCallName(*event.ToolCall)
		args = strings.TrimSpace(provider.ToolCallArguments(*event.ToolCall))
	}
	if args == "" {
		return name
	}
	return strings.TrimSpace(name + " " + toolArgsText(name, args))
}

func toolResultText(raw string) string {
	raw = strings.TrimSpace(raw)
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return compact(raw, 400)
	}
	if err := rawString(value, "error"); err != "" {
		return "error " + quote(compact(err, 180))
	}
	if sessions, ok := value["sessions"].([]any); ok {
		return formatSessions(sessions)
	}
	if assistant := rawString(value, "assistant"); assistant != "" {
		if slug := rawString(value, "slug"); slug != "" {
			return "completed " + slug
		}
		return compact(assistant, 300)
	}
	if output := rawString(value, "output"); output != "" {
		return compact(output, 300)
	}
	summary := strings.Join(nonEmpty(rawString(value, "status"), rawString(value, "slug"), rawString(value, "state")), " ")
	if summary == "" {
		return compact(formatMap(value), 300)
	}
	return summary
}

func toolArgsText(name, raw string) string {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return compact(raw, 200)
	}
	switch name {
	case "agent_spawn":
		return strings.Join(nonEmpty(rawString(value, "slug"), rawString(value, "acp_agent"), quote(compact(rawString(value, "title"), 80))), " ")
	case "agent_send":
		wait := ""
		if value["wait"] == true {
			wait = "wait=true"
		}
		return strings.Join(nonEmpty(rawString(value, "session"), quote(compact(rawString(value, "message"), 90)), wait), " ")
	default:
		return compact(formatMap(value), 240)
	}
}

func formatJSONSummary(value any) string {
	switch v := value.(type) {
	case map[string]any:
		if sessions, ok := v["sessions"].([]any); ok {
			return formatSessions(sessions)
		}
		return formatMap(v)
	case []any:
		parts := make([]string, 0, min(len(v), 6))
		for _, item := range v[:min(len(v), 6)] {
			parts = append(parts, formatJSONSummary(item))
		}
		if len(v) > 6 {
			parts = append(parts, fmt.Sprintf("+%d more", len(v)-6))
		}
		return strings.Join(parts, ", ")
	case string:
		return quote(v)
	default:
		return fmt.Sprint(v)
	}
}

func formatMap(value map[string]any) string {
	keys := preferredKeys(value)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		formatted := formatFieldValue(value[key])
		if formatted != "" {
			parts = append(parts, key+"="+formatted)
		}
	}
	return strings.Join(parts, "  ")
}

func preferredKeys(value map[string]any) []string {
	preferred := []string{"status", "slug", "session", "session_id", "acp_agent", "state", "stop_reason", "assistant", "error", "message", "cmd", "wait"}
	keys := make([]string, 0, len(value))
	seen := map[string]struct{}{}
	for _, key := range preferred {
		if _, ok := value[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	var rest []string
	for key := range value {
		if _, ok := seen[key]; !ok {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func formatFieldValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return quote(compact(v, 220))
	case bool, float64:
		return fmt.Sprint(v)
	case []any:
		return "[" + formatJSONSummary(v) + "]"
	case map[string]any:
		return "{" + formatMap(v) + "}"
	default:
		return fmt.Sprint(v)
	}
}

func formatSessions(sessions []any) string {
	if len(sessions) == 0 {
		return "sessions=none"
	}
	parts := make([]string, 0, min(len(sessions), 8))
	for _, item := range sessions[:min(len(sessions), 8)] {
		session, ok := item.(map[string]any)
		if !ok {
			continue
		}
		parts = append(parts, strings.TrimSpace(strings.Join([]string{
			stringField(session, "slug"),
			stringField(session, "state"),
			stringField(session, "assistant"),
		}, "  ")))
	}
	if len(sessions) > 8 {
		parts = append(parts, fmt.Sprintf("+%d more", len(sessions)-8))
	}
	return "sessions=" + strings.Join(parts, " | ")
}

func stringField(value map[string]any, key string) string {
	raw := rawString(value, key)
	if raw == "" {
		return ""
	}
	return key + "=" + quote(compact(raw, 120))
}

func rawString(value map[string]any, key string) string {
	raw, _ := value[key].(string)
	return strings.TrimSpace(raw)
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && value != `""` {
			out = append(out, value)
		}
	}
	return out
}

func quote(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \n\t{}[]:,") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

func sessionAgent(session chat.Session) string {
	if session.ACPAgent != "" {
		return normalizeAgent(session.ACPAgent)
	}
	if session.Runtime == "acp" {
		return "agent"
	}
	return ""
}

func normalizeAgent(agent string) string {
	return strings.ReplaceAll(strings.TrimSpace(agent), "_", "-")
}

var (
	headerStyle      = lipgloss.NewStyle().Padding(0, 1)
	brandStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8bd5ff"))
	sessionPillStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa3b5"))
	bodyStyle        = lipgloss.NewStyle().Padding(0, 1)
	statusStyle      = lipgloss.NewStyle().Padding(0, 1)
	inputBoxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#3a4150")).Padding(0, 1)
	userMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6deeb")).Background(lipgloss.Color("#303336")).Padding(1, 1)
	promptStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0c6d4"))
	messageTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6deeb"))
	toolTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa3b5"))
	errorTextStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	statusReadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
	statusBusyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af"))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#6f7787"))
)
