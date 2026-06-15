package threads

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/searchdb"
)

const (
	defaultSearchLimit  = 20
	maxSearchLimit      = 50
	maxSearchCandidates = 300
)

type SearchQuery struct {
	Query           string
	Roles           []string
	IncludeArchived bool
	Limit           int
}

type SearchResult struct {
	ThreadID        string
	ThreadSlug      string
	ThreadTitle     string
	ThreadStatus    string
	ThreadRuntime   string
	ParentID        string
	Archived        bool
	MessageSeq      int64
	Role            string
	Snippet         string
	HitCount        int
	UpdatedAt       time.Time
	LastAttentionAt time.Time
}

type Service struct {
	store searchdb.Querier
}

func NewService(store searchdb.Querier) *Service {
	return &Service{store: store}
}

type searchAccumulator struct {
	result    SearchResult
	bestScore float64
}

func (s *Service) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	match := ftsMatchQuery(query.Query)
	if match == "" {
		return nil, nil
	}
	limit := clampSearchLimit(query.Limit)
	candidateLimit := min(maxSearchCandidates, max(limit*8, 80))
	includeUser, includeAssistant := searchRoles(query.Roles)

	byThread := map[string]*searchAccumulator{}
	metadataRows, err := s.store.SearchThreadMetadata(ctx, searchdb.SearchThreadMetadataParams{
		Match:           match,
		IncludeArchived: boolInt(query.IncludeArchived),
		Limit:           int64(candidateLimit),
	})
	if err != nil {
		return nil, err
	}
	for _, row := range metadataRows {
		addMetadataRow(byThread, row)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if includeUser || includeAssistant {
		messageRows, err := s.store.SearchThreadMessages(ctx, searchdb.SearchThreadMessagesParams{
			Match:            match,
			IncludeUser:      boolInt(includeUser),
			IncludeAssistant: boolInt(includeAssistant),
			IncludeArchived:  boolInt(query.IncludeArchived),
			Limit:            int64(candidateLimit),
		})
		if err != nil {
			return nil, err
		}
		for _, row := range messageRows {
			addMessageRow(byThread, row)
		}
	}

	results := make([]SearchResult, 0, len(byThread))
	for _, item := range byThread {
		item.bestScore += hitCountBoost(item.result.HitCount)
		results = append(results, item.result)
	}
	sort.Slice(results, func(i, j int) bool {
		left := byThread[results[i].ThreadID].bestScore
		right := byThread[results[j].ThreadID].bestScore
		if left != right {
			return left > right
		}
		if !results[i].LastAttentionAt.Equal(results[j].LastAttentionAt) {
			return results[i].LastAttentionAt.After(results[j].LastAttentionAt)
		}
		return results[i].ThreadID < results[j].ThreadID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func addMessageRow(byThread map[string]*searchAccumulator, row searchdb.SearchThreadMessagesRow) {
	score := -row.Score * 1_000_000
	if row.Role == "user" {
		score += 1.5
	}
	addSearchHit(byThread, SearchResult{
		ThreadID:        row.ID,
		ThreadSlug:      row.Slug,
		ThreadTitle:     row.Title,
		ThreadStatus:    row.Status,
		ThreadRuntime:   row.Runtime,
		ParentID:        row.ParentID,
		Archived:        row.Archived != 0,
		MessageSeq:      row.Seq,
		Role:            row.Role,
		Snippet:         row.Snippet,
		UpdatedAt:       msToTime(row.UpdatedAtMs),
		LastAttentionAt: msToTime(row.LastAttentionAtMs),
	}, score)
}

func addMetadataRow(byThread map[string]*searchAccumulator, row searchdb.SearchThreadMetadataRow) {
	addSearchHit(byThread, SearchResult{
		ThreadID:        row.ID,
		ThreadSlug:      row.Slug,
		ThreadTitle:     row.Title,
		ThreadStatus:    row.Status,
		ThreadRuntime:   row.Runtime,
		ParentID:        row.ParentID,
		Archived:        row.Archived != 0,
		Snippet:         firstNonEmpty(row.TitleSnippet, row.SlugSnippet),
		UpdatedAt:       msToTime(row.UpdatedAtMs),
		LastAttentionAt: msToTime(row.LastAttentionAtMs),
	}, -row.Score*1_000_000+8)
}

func addSearchHit(byThread map[string]*searchAccumulator, result SearchResult, score float64) {
	item := byThread[result.ThreadID]
	if item == nil {
		item = &searchAccumulator{result: result, bestScore: score}
		byThread[result.ThreadID] = item
	}
	item.result.HitCount++
	if item.result.Snippet == "" || score > item.bestScore {
		item.bestScore = score
		item.result.MessageSeq = result.MessageSeq
		item.result.Role = result.Role
		item.result.Snippet = result.Snippet
	}
}

func ftsMatchQuery(query string) string {
	tokens := ftsTokens(query)
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len([]rune(token)) < 2 {
			continue
		}
		terms = append(terms, token+"*")
	}
	return strings.Join(terms, " AND ")
}

func ftsTokens(query string) []string {
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

func searchRoles(roles []string) (bool, bool) {
	if len(roles) == 0 {
		return true, true
	}
	var user, assistant bool
	for _, role := range roles {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "user":
			user = true
		case "assistant":
			assistant = true
		}
	}
	return user, assistant
}

func clampSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	return min(limit, maxSearchLimit)
}

func hitCountBoost(count int) float64 {
	if count <= 1 {
		return 0
	}
	return float64(min(count-1, 6)) * 0.3
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func msToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
