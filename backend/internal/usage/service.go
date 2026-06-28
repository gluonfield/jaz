package usage

import (
	"errors"
	"sort"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

const (
	DateLayout       = "2006-01-02"
	DefaultDailyDays = 30
	MaxDailyDays     = 365
)

var (
	ErrInvalidDays = errors.New("days must be a positive integer")
	ErrUnsupported = errors.New("usage statistics are not supported")
)

type Service struct {
	store storage.UsageEventStore
	now   func() time.Time
}

type DailyQuery struct {
	Days     int
	Location *time.Location
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
// type. Worker categories reuse their storage source-type keys verbatim.
const CategoryChat = "chat"

func usageCategory(sourceType string) string {
	if sourceType == "" {
		return CategoryChat
	}
	return sourceType
}

func (u UsageTotals) InputOutputTokens() int64 {
	return u.InputTokens + u.OutputTokens
}

func NewService(store storage.UsageEventStore) Service {
	return Service{store: store, now: time.Now}
}

func (s Service) Daily(query DailyQuery) ([]DailyBucket, error) {
	days, err := ValidateDays(query.Days)
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, ErrUnsupported
	}
	loc := query.Location
	if loc == nil {
		loc = time.Local
	}
	out, index, start := dailyBucketsAt(days, loc, s.currentTime())
	accs := make([]dayAccumulator, len(out))
	for i := range accs {
		accs[i] = newDayAccumulator()
	}
	events, err := s.store.UsageEventsSince(start.In(time.UTC))
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
		i, ok := index[event.CreatedAt.In(loc).Format(DateLayout)]
		if !ok {
			continue
		}
		acc := accs[i]
		AddDaily(&out[i].Usage, event.Usage)
		addUsageGroup(acc.categories, usageCategory(event.SourceType), event)
		if event.Runtime == storage.RuntimeACP {
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
	days, err := ValidateDays(query.Days)
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, ErrUnsupported
	}
	loc := query.Location
	if loc == nil {
		loc = time.Local
	}
	_, _, start := dailyBucketsAt(days, loc, s.currentTime())
	end := start.AddDate(0, 0, days)
	events, err := s.store.UsageEventsSince(start.In(time.UTC))
	if err != nil {
		return nil, err
	}
	groups := map[modelUsageKey]*usageGroup{}
	for _, event := range events {
		if event.Source != storage.UsageEventSourceTurn || event.Runtime != storage.RuntimeACP {
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

func ValidateDays(days int) (int, error) {
	if days == 0 {
		return DefaultDailyDays, nil
	}
	if days < 0 {
		return 0, ErrInvalidDays
	}
	if days > MaxDailyDays {
		return MaxDailyDays, nil
	}
	return days, nil
}

func NormalizeDays(days int) int {
	if days <= 0 {
		return DefaultDailyDays
	}
	if days > MaxDailyDays {
		return MaxDailyDays
	}
	return days
}

func (s Service) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func DailyBuckets(days int, loc *time.Location) ([]DailyBucket, map[string]int, time.Time) {
	return dailyBucketsAt(days, loc, time.Now())
}

func dailyBucketsAt(days int, loc *time.Location, now time.Time) ([]DailyBucket, map[string]int, time.Time) {
	days = NormalizeDays(days)
	now = now.In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -days+1)
	out := make([]DailyBucket, days)
	index := make(map[string]int, days)
	for i := range days {
		date := start.AddDate(0, 0, i).Format(DateLayout)
		out[i] = DailyBucket{Date: date}
		index[date] = i
	}
	return out, index, start
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
