package acp

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

type usageStore interface {
	AddUsage(string, storage.Usage) error
}

type usageReport struct {
	Snapshot storage.Usage
	Delta    storage.Usage
	Context  storage.Usage
}

func (m *Manager) recordRawUsage(job *Job, raw json.RawMessage) {
	m.recordUsageReport(job, usageReportFromRaw(raw))
}

func (m *Manager) recordUsage(job *Job, usage storage.Usage) {
	if usage.IsZero() {
		return
	}
	m.recordUsageReport(job, usageReport{Snapshot: usage})
}

// recordUsageReport persists usage on arrival. ACP adapters can report three
// different shapes: cumulative turn snapshots, per-request deltas, and live
// context size updates. Keeping those separate avoids both max-merging deltas
// and replaying cumulative totals as if they were turn-local.
func (m *Manager) recordUsageReport(job *Job, report usageReport) {
	if report.IsZero() {
		return
	}
	var write storage.Usage
	job.mu.Lock()
	if !report.Snapshot.IsZero() {
		prev := job.usage
		job.usage = mergeUsageSnapshot(prev, report.Snapshot)
		curr := job.usage
		if curr != prev {
			write = addUsageDelta(write, usageDelta(prev, curr))
		}
	}
	if !report.Delta.IsZero() {
		if !job.isDuplicateUsageDelta(report) {
			job.usage = addUsageDelta(job.usage, report.Delta)
			write = addUsageDelta(write, report.Delta)
		}
	} else {
		job.lastUsageDeltaSet = false
	}
	if !report.Context.IsZero() {
		job.usage = mergeUsageContext(job.usage, report.Context)
		write = mergeUsageContext(write, report.Context)
	}
	job.mu.Unlock()
	if write.IsZero() {
		return
	}
	store, ok := m.store.(usageStore)
	if !ok {
		return
	}
	if err := store.AddUsage(job.ID, write); err != nil {
		m.log.Error("persist acp usage failed", "session", job.ID, "error", err)
	}
}

func (j *Job) isDuplicateUsageDelta(report usageReport) bool {
	// Codex can repeat the same token_count notification; the ACP bridge only
	// carries lastTokenUsage, so consecutive identical delta reports are replays.
	duplicate := j.lastUsageDeltaSet &&
		j.lastUsageDelta == report.Delta &&
		j.lastUsageContext == report.Context
	j.lastUsageDelta = report.Delta
	j.lastUsageContext = report.Context
	j.lastUsageDeltaSet = true
	return duplicate
}

func addUsageDelta(current, delta storage.Usage) storage.Usage {
	current.InputTokens += delta.InputTokens
	current.CachedInputTokens += delta.CachedInputTokens
	current.CachedWriteTokens += delta.CachedWriteTokens
	current.OutputTokens += delta.OutputTokens
	current.ReasoningOutputTokens += delta.ReasoningOutputTokens
	if delta.TotalTokens > 0 {
		current.TotalTokens += delta.TotalTokens
	} else {
		current.TotalTokens += delta.ComponentTotal()
	}
	if delta.ContextTokens > 0 {
		current.ContextTokens = delta.ContextTokens
	}
	if delta.ContextWindowTokens > 0 {
		current.ContextWindowTokens = delta.ContextWindowTokens
	}
	return current
}

func mergeUsageContext(current, context storage.Usage) storage.Usage {
	if context.ContextTokens > 0 {
		current.ContextTokens = context.ContextTokens
	}
	if context.ContextWindowTokens > 0 {
		current.ContextWindowTokens = context.ContextWindowTokens
	}
	return current
}

// usageDelta is the write that advances the store from prev to curr. Counters
// are differences the store adds; context/window are the latest cumulative
// snapshot the store replaces. Context carries curr.LiveContextTokens() (never
// a per-delta value) so a counters-only delta can't clobber the context column
// with a component-total estimate of the increment.
func usageDelta(prev, curr storage.Usage) storage.Usage {
	return storage.Usage{
		InputTokens:           nonNegative(curr.InputTokens - prev.InputTokens),
		CachedInputTokens:     nonNegative(curr.CachedInputTokens - prev.CachedInputTokens),
		CachedWriteTokens:     nonNegative(curr.CachedWriteTokens - prev.CachedWriteTokens),
		OutputTokens:          nonNegative(curr.OutputTokens - prev.OutputTokens),
		ReasoningOutputTokens: nonNegative(curr.ReasoningOutputTokens - prev.ReasoningOutputTokens),
		TotalTokens:           nonNegative(curr.TotalTokens - prev.TotalTokens),
		ContextTokens:         curr.LiveContextTokens(),
		ContextWindowTokens:   curr.ContextWindowTokens,
	}
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// usageFromRaw parses one adapter message into disjoint usage components.
// Fragments are collected verbatim and normalized exactly once here — a
// message is internally coherent (one provider vocabulary), while fragments
// in isolation are not.
func usageFromRaw(raw json.RawMessage) storage.Usage {
	return usageReportFromRaw(raw).Usage()
}

func usageReportFromRaw(raw json.RawMessage) usageReport {
	if len(raw) == 0 {
		return usageReport{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return usageReport{}
	}
	if kind, ok := fields["sessionUpdate"]; ok {
		var name string
		if json.Unmarshal(kind, &name) == nil && name == "usage_update" {
			report := usageReport{
				Context: storage.Usage{
					ContextTokens:       firstIntField(fields, "used"),
					ContextWindowTokens: firstIntField(fields, "size"),
				},
			}
			if meta, ok := fields["_meta"]; ok {
				report.Merge(usageReportFromRaw(meta))
			}
			return report
		}
	}
	report := usageReport{Snapshot: usageSnapshotFromRaw(raw)}
	if !report.Snapshot.Countable() {
		report.Context = mergeUsageContext(report.Context, report.Snapshot)
		report.Snapshot = storage.Usage{}
	}
	report.Delta = lastTokenUsageDelta(raw)
	if report.Snapshot.Countable() {
		report.Delta = storage.Usage{}
	}
	return report
}

func (r usageReport) IsZero() bool {
	return r.Snapshot.IsZero() && r.Delta.IsZero() && r.Context.IsZero()
}

func (r usageReport) Usage() storage.Usage {
	usage := r.Snapshot
	if !r.Delta.IsZero() {
		usage = addUsageDelta(usage, r.Delta)
	}
	return mergeUsageContext(usage, r.Context)
}

func (r *usageReport) Merge(next usageReport) {
	if !next.Snapshot.IsZero() {
		r.Snapshot = mergeUsageSnapshot(r.Snapshot, next.Snapshot)
	}
	if !next.Delta.IsZero() {
		r.Delta = addUsageDelta(r.Delta, next.Delta)
	}
	r.Context = mergeUsageContext(r.Context, next.Context)
}

func usageSnapshotFromRaw(raw json.RawMessage) storage.Usage {
	usage, inclusive := usageSnapshotFragment(raw)
	return dropTotalOnly(normalizeDisjoint(usage, inclusive))
}

// usageSnapshotFragment recursively collects cumulative snapshot fields
// without normalizing. The boolean reports OpenAI-style vocabulary, whose
// input count includes cache reads.
func usageSnapshotFragment(raw json.RawMessage) (storage.Usage, bool) {
	if len(raw) == 0 {
		return storage.Usage{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return storage.Usage{}, false
	}
	usage, inclusive := usageFromFields(fields)
	merge := func(nested storage.Usage, nestedInclusive bool) {
		usage = mergeUsageSnapshot(usage, nested)
		inclusive = inclusive || nestedInclusive
	}
	for _, key := range []string{
		"usage", "tokenUsage", "token_usage",
		"prompt_tokens_details", "promptTokensDetails", "input_tokens_details", "inputTokensDetails",
		"completion_tokens_details", "completionTokensDetails", "output_tokens_details", "outputTokensDetails",
		"_meta", "meta", "metadata",
	} {
		if nested, ok := fields[key]; ok {
			merge(usageSnapshotFragment(nested))
		}
	}
	if nested, ok := fields["tokens"]; ok {
		merge(usageFromTokens(nested), false)
	}
	return usage, inclusive
}

func lastTokenUsageDelta(raw json.RawMessage) storage.Usage {
	if len(raw) == 0 {
		return storage.Usage{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return storage.Usage{}
	}
	var out storage.Usage
	for _, key := range []string{"lastTokenUsage", "last_token_usage"} {
		if nested, ok := fields[key]; ok {
			out = addUsageDelta(out, usageSnapshotFromRaw(nested))
		}
	}
	for _, key := range []string{"_meta", "meta", "metadata", "info", "payload"} {
		if nested, ok := fields[key]; ok {
			out = addUsageDelta(out, lastTokenUsageDelta(nested))
		}
	}
	return out
}

func usageFromFields(fields map[string]json.RawMessage) (storage.Usage, bool) {
	var usage storage.Usage
	usage.InputTokens = firstIntField(fields, "input_tokens", "inputTokens", "input", "prompt_tokens", "promptTokens")
	inclusive := hasField(fields, "prompt_tokens", "promptTokens")
	usage.OutputTokens = firstIntField(fields, "output_tokens", "outputTokens", "output", "completion_tokens", "completionTokens")
	// Disjoint cache-read vocabulary (Anthropic/ACP adapters) first; the
	// OpenAI family marks the payload as inclusive.
	usage.CachedInputTokens = firstIntField(fields, "cached_read_tokens", "cachedReadTokens", "cache_read_tokens", "cacheReadTokens")
	if usage.CachedInputTokens == 0 {
		usage.CachedInputTokens = firstIntField(fields, "cache_read_input_tokens", "cacheReadInputTokens")
	}
	if usage.CachedInputTokens == 0 {
		if read := firstIntField(fields,
			"cached_tokens", "cachedTokens", "cached_input_tokens", "cachedInputTokens",
		); read > 0 {
			usage.CachedInputTokens = read
			inclusive = true
		}
	}
	usage.CachedWriteTokens = firstIntField(fields,
		"cache_creation_input_tokens", "cacheCreationInputTokens",
		"cached_write_tokens", "cachedWriteTokens", "cache_write_tokens", "cacheWriteTokens")
	usage.ReasoningOutputTokens = firstIntField(fields,
		"reasoning_output_tokens", "reasoningOutputTokens",
		"reasoning_tokens", "reasoningTokens",
		"thought_tokens", "thoughtTokens")
	usage.TotalTokens = firstIntField(fields, "total_tokens", "totalTokens")
	usage.ContextTokens = firstIntField(fields, "context_tokens", "contextTokens", "context_used_tokens", "contextUsedTokens")
	return usage, inclusive
}

// normalizeDisjoint converts inclusive counting (cache reads inside the
// input count) to the disjoint convention storage.Usage uses, and fills a
// missing total from the components. A reported total settles the question
// arithmetically; without one, the key vocabulary decides.
func normalizeDisjoint(usage storage.Usage, inclusiveVocabulary bool) storage.Usage {
	read := usage.CachedInputTokens
	write := usage.CachedWriteTokens
	cached := read + write
	confirmedDisjoint := usage.TotalTokens > 0 && usage.TotalTokens == usage.ComponentTotal()
	confirmedInclusive := usage.TotalTokens > 0 && usage.TotalTokens == usage.InputTokens+usage.OutputTokens
	if cached > 0 && cached <= usage.InputTokens && !confirmedDisjoint &&
		(confirmedInclusive || (usage.TotalTokens == 0 && inclusiveVocabulary)) {
		usage.InputTokens -= cached
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.ComponentTotal()
	}
	return usage
}

func dropTotalOnly(usage storage.Usage) storage.Usage {
	if usage.TotalTokens == 0 {
		return usage
	}
	if usage.InputTokens == 0 && usage.CachedInputTokens == 0 && usage.CachedWriteTokens == 0 &&
		usage.OutputTokens == 0 && usage.ReasoningOutputTokens == 0 &&
		usage.ContextTokens == 0 && usage.ContextWindowTokens == 0 {
		return storage.Usage{}
	}
	return usage
}

// usageFromTokens parses the `tokens: {input, output, cache: {read, write}}`
// shape; its explicit read/write split marks it as disjoint vocabulary.
func usageFromTokens(raw json.RawMessage) storage.Usage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return storage.Usage{}
	}
	usage := storage.Usage{
		InputTokens:           firstIntField(fields, "input", "input_tokens", "inputTokens"),
		OutputTokens:          firstIntField(fields, "output", "output_tokens", "outputTokens"),
		ReasoningOutputTokens: firstIntField(fields, "reasoning", "reasoning_tokens", "reasoningTokens", "thought_tokens", "thoughtTokens"),
	}
	if nested, ok := fields["cache"]; ok {
		var cache map[string]json.RawMessage
		if json.Unmarshal(nested, &cache) == nil {
			usage.CachedInputTokens = firstIntField(cache, "read", "cached", "cached_tokens", "cachedTokens", "cache_read_tokens", "cacheReadTokens")
			usage.CachedWriteTokens = firstIntField(cache, "write", "creation", "cache_write_tokens", "cacheWriteTokens")
		}
	}
	return usage
}

func hasField(fields map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func firstIntField(fields map[string]json.RawMessage, keys ...string) int64 {
	for _, key := range keys {
		value, ok := intField(fields, key)
		if ok {
			return value
		}
	}
	return 0
}

func intField(fields map[string]json.RawMessage, key string) (int64, bool) {
	raw, ok := fields[key]
	if !ok {
		return 0, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return 0, false
	}
	switch v := value.(type) {
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, true
		}
		if f, err := v.Float64(); err == nil {
			return int64(f), true
		}
	case float64:
		return int64(v), true
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

func mergeUsageSnapshot(current, next storage.Usage) storage.Usage {
	if next.InputTokens > current.InputTokens {
		current.InputTokens = next.InputTokens
	}
	if next.CachedInputTokens > current.CachedInputTokens {
		current.CachedInputTokens = next.CachedInputTokens
	}
	if next.CachedWriteTokens > current.CachedWriteTokens {
		current.CachedWriteTokens = next.CachedWriteTokens
	}
	if next.OutputTokens > current.OutputTokens {
		current.OutputTokens = next.OutputTokens
	}
	if next.ReasoningOutputTokens > current.ReasoningOutputTokens {
		current.ReasoningOutputTokens = next.ReasoningOutputTokens
	}
	if next.TotalTokens > current.TotalTokens {
		current.TotalTokens = next.TotalTokens
	}
	// Context is a live snapshot, not a counter: the latest report wins even
	// when smaller (mid-turn compaction shrinks it).
	if next.ContextTokens > 0 {
		current.ContextTokens = next.ContextTokens
	}
	if next.ContextWindowTokens > 0 {
		current.ContextWindowTokens = next.ContextWindowTokens
	}
	return current
}
