package usage

import (
	"errors"
	"sort"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

var ErrUnsupported = errors.New("usage statistics are not supported")

type Service struct {
	store     storage.UsageEventStore
	workspace func() string
	now       func() time.Time
}

type DailyBucket struct {
	Date         string
	Usage        UsageTotals
	Models       []ModelUsage
	Categories   []CategoryUsage
	SessionCount int
}

type UsageTotals struct {
	InputTokens           int64
	CachedInputTokens     int64
	CachedWriteTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
}

type ModelUsage struct {
	Agent         string
	ModelProvider string
	Model         string
	Usage         UsageTotals
	SessionCount  int
}

// CategoryUsage splits usage by the originating session's activity. Categories
// partition the daily total: every turn event lands in exactly one. Unlike
// ModelUsage there is no per-category session count: nothing consumes one, and
// categories have no dedicated endpoint where it would be a contract.
type CategoryUsage struct {
	Category string
	Usage    UsageTotals
}

// CategoryChat is the category for interactive sessions, which carry no source
// type. Other categories reuse their storage source-type keys verbatim, except
// that a bot's subtasks are the bot's own work.
const CategoryChat = "chat"

func usageCategory(sourceType string) string {
	switch sourceType {
	case "":
		return CategoryChat
	case storage.SourceBotWorker, storage.SourceBotMember:
		return storage.SourceBot
	}
	return sourceType
}

func (u UsageTotals) InputOutputTokens() int64 {
	return max(0, u.InputTokens-u.CachedInputTokens) + u.OutputTokens
}

func NewService(store storage.UsageEventStore) Service {
	return Service{store: store, now: time.Now}
}

func (s Service) WithWorkspace(fn func() string) Service {
	s.workspace = fn
	return s
}

func (s Service) usageEvents(start, end time.Time) ([]storage.UsageEvent, error) {
	if s.workspace != nil {
		if wsStore, ok := s.store.(storage.WorkspaceUsageEventStore); ok {
			return wsStore.UsageEventsForWorkspace(s.workspace(), start, end)
		}
	}
	return s.store.UsageEvents(start, end)
}

func (s Service) Daily(query DailyQuery) ([]DailyBucket, error) {
	window, err := query.window(s.currentTime())
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, ErrUnsupported
	}
	out := []DailyBucket{}
	index := map[string]int{}
	for date := window.first; date.Before(window.end); date = date.AddDate(0, 0, 1) {
		key := date.Format(DateLayout)
		index[key] = len(out)
		out = append(out, DailyBucket{Date: key})
	}
	accs := make([]dayAccumulator, len(out))
	for i := range accs {
		accs[i] = newDayAccumulator()
	}
	start, end := window.bounds()
	events, err := s.usageEvents(start, end)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.Source != storage.UsageEventSourceTurn {
			continue
		}
		if event.CreatedAt.Before(start) {
			continue
		}
		i, ok := index[event.CreatedAt.In(window.location).Format(DateLayout)]
		if !ok {
			continue
		}
		acc := accs[i]
		AddDaily(&out[i].Usage, event.Usage)
		addUsageGroup(acc.categories, usageCategory(event.SourceType), event)
		if event.Runtime == storage.RuntimeACP || event.Runtime == "cloud" {
			addUsageGroup(acc.models, modelKey(event), event)
		}
		if event.Usage.Countable() {
			acc.sessions[event.SessionID] = struct{}{}
		}
	}
	for i := range out {
		out[i].SessionCount = len(accs[i].sessions)
		out[i].Models = modelUsageFromGroups(accs[i].models)
		out[i].Categories = categoryUsageFromGroups(accs[i].categories)
	}
	return out, nil
}

func (s Service) Models(query DailyQuery) ([]ModelUsage, error) {
	window, err := query.window(s.currentTime())
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, ErrUnsupported
	}
	start, end := window.bounds()
	events, err := s.usageEvents(start, end)
	if err != nil {
		return nil, err
	}
	groups := map[modelUsageKey]*usageGroup{}
	for _, event := range events {
		if event.Source != storage.UsageEventSourceTurn || (event.Runtime != storage.RuntimeACP && event.Runtime != "cloud") {
			continue
		}
		if event.CreatedAt.Before(start) || !event.CreatedAt.Before(end) {
			continue
		}
		addUsageGroup(groups, modelKey(event), event)
	}
	return modelUsageFromGroups(groups), nil
}

func modelUsageFromGroups(groups map[modelUsageKey]*usageGroup) []ModelUsage {
	out := make([]ModelUsage, 0, len(groups))
	for key, group := range groups {
		out = append(out, ModelUsage{
			Agent:         key.agent,
			ModelProvider: key.provider,
			Model:         key.model,
			Usage:         group.usage,
			SessionCount:  len(group.sessionIDs),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Usage.InputOutputTokens()
		right := out[j].Usage.InputOutputTokens()
		if left != right {
			return left > right
		}
		if out[i].Agent != out[j].Agent {
			return out[i].Agent < out[j].Agent
		}
		if out[i].ModelProvider != out[j].ModelProvider {
			return out[i].ModelProvider < out[j].ModelProvider
		}
		return out[i].Model < out[j].Model
	})
	return out
}

func categoryUsageFromGroups(groups map[string]*usageGroup) []CategoryUsage {
	out := make([]CategoryUsage, 0, len(groups))
	for key, group := range groups {
		out = append(out, CategoryUsage{
			Category: key,
			Usage:    group.usage,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Usage.InputOutputTokens()
		right := out[j].Usage.InputOutputTokens()
		if left != right {
			return left > right
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func (s Service) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func AddDaily(total *UsageTotals, event storage.Usage) {
	total.InputTokens += event.InputTokens
	total.CachedInputTokens += event.CachedInputTokens
	total.CachedWriteTokens += event.CachedWriteTokens
	total.OutputTokens += event.OutputTokens
	total.ReasoningOutputTokens += event.ReasoningOutputTokens
}

// dayAccumulator collects one day's events: distinct sessions for the day's
// count, plus the model and category breakdowns keyed by identity.
type dayAccumulator struct {
	sessions   map[string]struct{}
	models     map[modelUsageKey]*usageGroup
	categories map[string]*usageGroup
}

func newDayAccumulator() dayAccumulator {
	return dayAccumulator{
		sessions:   map[string]struct{}{},
		models:     map[modelUsageKey]*usageGroup{},
		categories: map[string]*usageGroup{},
	}
}

// usageGroup accumulates token totals and distinct sessions for one key. The
// model and category breakdowns are the same group-by-then-rank shape over
// different keys, so they share this accumulator; the key carries identity.
type usageGroup struct {
	usage      UsageTotals
	sessionIDs map[string]struct{}
}

func addUsageGroup[K comparable](groups map[K]*usageGroup, key K, event storage.UsageEvent) {
	group := groups[key]
	if group == nil {
		group = &usageGroup{sessionIDs: map[string]struct{}{}}
		groups[key] = group
	}
	AddDaily(&group.usage, event.Usage)
	if event.Usage.Countable() {
		group.sessionIDs[event.SessionID] = struct{}{}
	}
}

type modelUsageKey struct {
	agent    string
	provider string
	model    string
}

func modelKey(event storage.UsageEvent) modelUsageKey {
	return modelUsageKey{agent: event.Agent, provider: event.ModelProvider, model: event.Model}
}
