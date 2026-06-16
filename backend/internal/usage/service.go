package usage

import (
	"errors"
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

func NewService(store storage.UsageEventStore) Service {
	return Service{store: store, now: time.Now}
}

func (s Service) Daily(query DailyQuery) ([]storage.DailyUsage, error) {
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
	sessionIDs := make([]map[string]struct{}, len(out))
	for i := range sessionIDs {
		sessionIDs[i] = map[string]struct{}{}
	}
	events, err := s.store.UsageEventsSince(start.In(time.UTC))
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.CreatedAt.Before(start) {
			continue
		}
		i, ok := index[event.CreatedAt.In(loc).Format(DateLayout)]
		if !ok {
			continue
		}
		AddDaily(&out[i].Usage, event.Usage)
		if event.Usage.Countable() {
			sessionIDs[i][event.SessionID] = struct{}{}
		}
	}
	for i := range out {
		out[i].SessionCount = len(sessionIDs[i])
	}
	return out, nil
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

func DailyBuckets(days int, loc *time.Location) ([]storage.DailyUsage, map[string]int, time.Time) {
	return dailyBucketsAt(days, loc, time.Now())
}

func dailyBucketsAt(days int, loc *time.Location, now time.Time) ([]storage.DailyUsage, map[string]int, time.Time) {
	days = NormalizeDays(days)
	now = now.In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -days+1)
	out := make([]storage.DailyUsage, days)
	index := make(map[string]int, days)
	for i := range days {
		date := start.AddDate(0, 0, i).Format(DateLayout)
		out[i] = storage.DailyUsage{Date: date}
		index[date] = i
	}
	return out, index, start
}

func AddDaily(total *storage.Usage, event storage.Usage) {
	eventTotal := event.TotalTokens
	if eventTotal == 0 {
		eventTotal = event.ComponentTotal()
	}
	total.InputTokens += event.InputTokens
	total.CachedInputTokens += event.CachedInputTokens
	total.CachedWriteTokens += event.CachedWriteTokens
	total.OutputTokens += event.OutputTokens
	total.ReasoningOutputTokens += event.ReasoningOutputTokens
	total.TotalTokens += eventTotal
}
