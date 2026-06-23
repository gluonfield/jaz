package threads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/wins/jaz/backend/internal/storage"
)

const (
	defaultContextLimit    = 16
	maxContextLimit        = 60
	defaultSearchContext   = 0
	maxSearchContext       = 4
	defaultMaxTextChars    = 2000
	maxTextChars           = 8000
	defaultMaxToolChars    = 300
	maxContextToolChars    = 1000
	IncludeToolsNone       = "none"
	IncludeToolsSummary    = "summary"
	IncludeToolsCompressed = "compressed"
)

type ContextStore interface {
	LoadSession(string) (storage.Session, error)
	LoadMessageRecords(string) ([]storage.Message, error)
}

type ContextRequest struct {
	Session      string `json:"session" jsonschema:"Jaz thread id or thread slug"`
	Query        string `json:"query,omitempty" jsonschema:"optional search query; returns matching message neighborhoods when set"`
	Limit        int    `json:"limit,omitempty" jsonschema:"maximum returned messages, 1-60; defaults to 16"`
	Context      int    `json:"context,omitempty" jsonschema:"messages before and after each query hit, 0-4; defaults to 0"`
	BeforeSeq    int64  `json:"before_seq,omitempty" jsonschema:"return the page before this message sequence"`
	AfterSeq     int64  `json:"after_seq,omitempty" jsonschema:"return the page after this message sequence"`
	AroundSeq    int64  `json:"around_seq,omitempty" jsonschema:"return a page centered near this message sequence"`
	IncludeTools string `json:"include_tools,omitempty" jsonschema:"none, summary, or compressed; defaults to summary"`
	MaxToolChars int    `json:"max_tool_chars,omitempty" jsonschema:"per-tool detail limit when include_tools is compressed; max 1000"`
	MaxTextChars int    `json:"max_text_chars,omitempty" jsonschema:"per-message text limit; max 8000; defaults to 2000"`
}

type ContextResponse struct {
	Session       ContextSession   `json:"session"`
	Mode          string           `json:"mode"`
	Query         string           `json:"query,omitempty"`
	Messages      []ContextMessage `json:"messages"`
	MatchCount    int              `json:"match_count,omitempty"`
	ToolCounts    map[string]int   `json:"tool_counts,omitempty"`
	HasMoreBefore bool             `json:"has_more_before,omitempty"`
	HasMoreAfter  bool             `json:"has_more_after,omitempty"`
	NextBeforeSeq int64            `json:"next_before_seq,omitempty"`
	NextAfterSeq  int64            `json:"next_after_seq,omitempty"`
	Truncated     bool             `json:"truncated,omitempty"`
}

type ContextSession struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	Title        string    `json:"title,omitempty"`
	ParentID     string    `json:"parent_id,omitempty"`
	Status       string    `json:"status"`
	Runtime      string    `json:"runtime"`
	Agent        string    `json:"agent,omitempty"`
	MessageCount int       `json:"message_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ContextMessage struct {
	Seq       int64         `json:"seq"`
	Role      string        `json:"role"`
	Text      string        `json:"text,omitempty"`
	Tools     []ContextTool `json:"tools,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	Matched   bool          `json:"matched,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
	search    string
}

type ContextTool struct {
	Name      string `json:"name,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type contextOptions struct {
	limit        int
	radius       int
	includeTools string
	maxToolChars int
	maxTextChars int
}

func (s *Service) Context(ctx context.Context, req ContextRequest) (ContextResponse, error) {
	if err := ctx.Err(); err != nil {
		return ContextResponse{}, err
	}
	if s.context == nil {
		return ContextResponse{}, errors.New("thread context store is not configured")
	}
	sessionRef := strings.TrimSpace(req.Session)
	if sessionRef == "" {
		return ContextResponse{}, errors.New("session is required")
	}
	if queryHasCursor(req) {
		return ContextResponse{}, errors.New("query cannot be combined with before_seq, after_seq, or around_seq")
	}
	if countCursors(req) > 1 {
		return ContextResponse{}, errors.New("use only one of before_seq, after_seq, or around_seq")
	}
	opts, err := contextOptionsFromRequest(req)
	if err != nil {
		return ContextResponse{}, err
	}
	session, err := s.context.LoadSession(sessionRef)
	if err != nil {
		return ContextResponse{}, err
	}
	records, err := s.context.LoadMessageRecords(session.ID)
	if err != nil {
		return ContextResponse{}, err
	}
	visible := visibleContextRecords(records, opts)
	response := ContextResponse{
		Session:    contextSession(session, len(visible)),
		Mode:       "tail",
		Messages:   []ContextMessage{},
		ToolCounts: ToolCounts(records),
	}
	query := strings.TrimSpace(req.Query)
	if query != "" {
		return contextQueryResponse(response, visible, query, opts), nil
	}
	return contextPageResponse(response, visible, req, opts), nil
}

func queryHasCursor(req ContextRequest) bool {
	return strings.TrimSpace(req.Query) != "" && countCursors(req) > 0
}

func countCursors(req ContextRequest) int {
	count := 0
	for _, value := range []int64{req.BeforeSeq, req.AfterSeq, req.AroundSeq} {
		if value > 0 {
			count++
		}
	}
	return count
}

func contextOptionsFromRequest(req ContextRequest) (contextOptions, error) {
	limit, err := positiveLimit(req.Limit, defaultContextLimit, maxContextLimit, "limit")
	if err != nil {
		return contextOptions{}, err
	}
	maxText, err := positiveLimit(req.MaxTextChars, defaultMaxTextChars, maxTextChars, "max_text_chars")
	if err != nil {
		return contextOptions{}, err
	}
	maxTool, err := positiveLimit(req.MaxToolChars, defaultMaxToolChars, maxContextToolChars, "max_tool_chars")
	if err != nil {
		return contextOptions{}, err
	}
	radius := req.Context
	if radius == 0 {
		radius = defaultSearchContext
	}
	if radius < 0 || radius > maxSearchContext {
		return contextOptions{}, fmt.Errorf("context must be between 0 and %d", maxSearchContext)
	}
	includeTools := strings.ToLower(strings.TrimSpace(req.IncludeTools))
	if includeTools == "" {
		includeTools = IncludeToolsSummary
	}
	switch includeTools {
	case IncludeToolsNone, IncludeToolsSummary, IncludeToolsCompressed:
	default:
		return contextOptions{}, errors.New("include_tools must be none, summary, or compressed")
	}
	return contextOptions{
		limit:        limit,
		radius:       radius,
		includeTools: includeTools,
		maxToolChars: maxTool,
		maxTextChars: maxText,
	}, nil
}

func positiveLimit(value, fallback, maxValue int, name string) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be non-negative", name)
	}
	if value > maxValue {
		return maxValue, nil
	}
	return value, nil
}

func contextSession(session storage.Session, count int) ContextSession {
	agent := ""
	if session.RuntimeRef != nil {
		agent = session.RuntimeRef.Agent
	}
	if agent == "" {
		agent = session.ModelProvider
	}
	return ContextSession{
		ID:           session.ID,
		Slug:         session.Slug,
		Title:        session.Title,
		ParentID:     session.ParentID,
		Status:       session.Status,
		Runtime:      session.Runtime,
		Agent:        agent,
		MessageCount: count,
		UpdatedAt:    session.UpdatedAt,
	}
}

func visibleContextRecords(records []storage.Message, opts contextOptions) []ContextMessage {
	out := make([]ContextMessage, 0, len(records))
	for _, record := range records {
		if record.Role != "user" && record.Role != "assistant" {
			continue
		}
		message := ContextMessage{
			Seq:       record.Seq,
			Role:      record.Role,
			CreatedAt: record.CreatedAt,
		}
		message.Text, message.Truncated = clampText(transcriptText(record), opts.maxTextChars)
		message.Tools = contextTools(record.Blocks, opts)
		message.search = recordSearchText(record)
		if message.Text == "" && len(message.Tools) == 0 {
			continue
		}
		out = append(out, message)
	}
	return out
}

func contextPageResponse(response ContextResponse, visible []ContextMessage, req ContextRequest, opts contextOptions) ContextResponse {
	start, end := tailWindow(len(visible), opts.limit)
	switch {
	case req.BeforeSeq > 0:
		response.Mode = "before"
		end = firstIndexAtOrAfter(visible, req.BeforeSeq)
		start = max(0, end-opts.limit)
	case req.AfterSeq > 0:
		response.Mode = "after"
		start = firstIndexAfter(visible, req.AfterSeq)
		end = min(len(visible), start+opts.limit)
	case req.AroundSeq > 0:
		response.Mode = "around"
		start, end = aroundWindow(visible, req.AroundSeq, opts.limit)
	}
	response.Messages = append([]ContextMessage(nil), visible[start:end]...)
	response.HasMoreBefore = start > 0
	response.HasMoreAfter = end < len(visible)
	if response.HasMoreBefore && len(response.Messages) > 0 {
		response.NextBeforeSeq = response.Messages[0].Seq
	}
	if response.HasMoreAfter && len(response.Messages) > 0 {
		response.NextAfterSeq = response.Messages[len(response.Messages)-1].Seq
	}
	return response
}

func contextQueryResponse(response ContextResponse, visible []ContextMessage, query string, opts contextOptions) ContextResponse {
	response.Mode = "query"
	response.Query = query
	tokens := searchTokens(query)
	if len(tokens) == 0 {
		response.Messages = []ContextMessage{}
		return response
	}
	selected := map[int]bool{}
	matched := map[int]bool{}
	for index, message := range visible {
		if !matchesAllTokens(contextSearchText(message), tokens) {
			continue
		}
		response.MatchCount++
		matched[index] = true
		if len(selected) >= opts.limit {
			response.Truncated = true
			continue
		}
		for i := max(0, index-opts.radius); i <= min(len(visible)-1, index+opts.radius); i++ {
			selected[i] = true
		}
		if len(selected) >= opts.limit {
			response.Truncated = true
		}
	}
	for i, message := range visible {
		if !selected[i] {
			continue
		}
		message.Matched = matched[i]
		response.Messages = append(response.Messages, message)
		if len(response.Messages) == opts.limit {
			break
		}
	}
	return response
}

func tailWindow(count, limit int) (int, int) {
	start := max(0, count-limit)
	return start, count
}

func aroundWindow(messages []ContextMessage, seq int64, limit int) (int, int) {
	center := firstIndexAtOrAfter(messages, seq)
	if center == len(messages) {
		center = max(0, len(messages)-1)
	}
	start := max(0, center-limit/2)
	end := min(len(messages), start+limit)
	start = max(0, end-limit)
	return start, end
}

func firstIndexAtOrAfter(messages []ContextMessage, seq int64) int {
	for i, message := range messages {
		if message.Seq >= seq {
			return i
		}
	}
	return len(messages)
}

func firstIndexAfter(messages []ContextMessage, seq int64) int {
	for i, message := range messages {
		if message.Seq > seq {
			return i
		}
	}
	return len(messages)
}

func transcriptText(record storage.Message) string {
	if text := strings.TrimSpace(record.Content); text != "" {
		return text
	}
	var parts []string
	for _, block := range record.Blocks {
		if block.Type != storage.BlockTypeText {
			continue
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func contextTools(blocks []storage.Block, opts contextOptions) []ContextTool {
	if opts.includeTools == IncludeToolsNone {
		return nil
	}
	var out []ContextTool
	for _, block := range blocks {
		if block.Type != storage.BlockTypeTool {
			continue
		}
		tool := ContextTool{Name: block.Name}
		if opts.includeTools == IncludeToolsCompressed {
			tool.Detail, tool.Truncated = clampText(compressedToolDetail(block), opts.maxToolChars)
		}
		out = append(out, tool)
	}
	return out
}

func compressedToolDetail(block storage.Block) string {
	detail := strings.TrimSpace(block.InputJSON)
	if result := strings.TrimSpace(block.Result); result != "" {
		if detail != "" {
			detail += " -> "
		}
		detail += result
	}
	return strings.Join(strings.Fields(detail), " ")
}

func ToolCounts(records []storage.Message) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		for _, block := range record.Blocks {
			if block.Type != storage.BlockTypeTool {
				continue
			}
			name := block.Name
			if name == "" {
				name = "unknown"
			}
			counts[name]++
		}
	}
	return counts
}

func contextSearchText(message ContextMessage) string {
	if message.search != "" {
		return message.search
	}
	var parts []string
	parts = append(parts, message.Text)
	for _, tool := range message.Tools {
		parts = append(parts, tool.Name, tool.Detail)
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func recordSearchText(record storage.Message) string {
	parts := []string{transcriptText(record)}
	for _, block := range record.Blocks {
		if block.Type != storage.BlockTypeTool {
			continue
		}
		parts = append(parts, block.Name, compressedToolDetail(block))
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func matchesAllTokens(text string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func searchTokens(query string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(current.String()))
		current.Reset()
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func clampText(text string, maxChars int) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || maxChars <= 0 {
		return "", text != ""
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text, false
	}
	return string(runes[:maxChars]) + "...", true
}
