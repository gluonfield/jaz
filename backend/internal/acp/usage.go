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

func (m *Manager) recordUsage(job *Job, usage storage.Usage) {
	if usage.IsZero() {
		return
	}
	job.mu.Lock()
	job.usage = mergeUsageSnapshot(job.usage, usage)
	job.mu.Unlock()
}

func (m *Manager) persistUsage(job *Job) {
	if m == nil || m.store == nil {
		return
	}
	store, ok := m.store.(usageStore)
	if !ok {
		return
	}
	job.mu.Lock()
	usage := job.usage
	job.usage = storage.Usage{}
	job.mu.Unlock()
	if usage.IsZero() {
		return
	}
	_ = store.AddUsage(job.ID, usage)
}

// usageFromRaw parses one adapter message into disjoint usage components.
// Fragments are collected verbatim and normalized exactly once here — a
// message is internally coherent (one provider vocabulary), while fragments
// in isolation are not.
func usageFromRaw(raw json.RawMessage) storage.Usage {
	usage, inclusive := usageFragment(raw)
	return normalizeDisjoint(usage, inclusive)
}

// usageFragment recursively collects token fields without normalizing.
// The boolean reports whether any matched key belongs to the OpenAI
// vocabulary (prompt_tokens / cached_tokens families), whose input counts
// include cache reads.
func usageFragment(raw json.RawMessage) (storage.Usage, bool) {
	if len(raw) == 0 {
		return storage.Usage{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return storage.Usage{}, false
	}
	// usage_update session updates (claude-agent-acp, codex-acp) carry the
	// live context fill and window size; "used"/"size" are only safe to read
	// under that discriminator. codex-acp additionally relays the last
	// request's token components in _meta.
	if kind, ok := fields["sessionUpdate"]; ok {
		var name string
		if json.Unmarshal(kind, &name) == nil && name == "usage_update" {
			usage := storage.Usage{
				ContextTokens:       firstIntField(fields, "used"),
				ContextWindowTokens: firstIntField(fields, "size"),
			}
			if meta, ok := fields["_meta"]; ok {
				nested, inclusive := usageFragment(meta)
				return mergeUsageSnapshot(usage, nested), inclusive
			}
			return usage, false
		}
	}
	usage, inclusive := usageFromFields(fields)
	merge := func(nested storage.Usage, nestedInclusive bool) {
		usage = mergeUsageSnapshot(usage, nested)
		inclusive = inclusive || nestedInclusive
	}
	for _, key := range []string{
		"usage", "tokenUsage", "token_usage", "lastTokenUsage", "last_token_usage",
		"prompt_tokens_details", "promptTokensDetails", "input_tokens_details", "inputTokensDetails",
		"completion_tokens_details", "completionTokensDetails", "output_tokens_details", "outputTokensDetails",
		"_meta", "meta", "metadata",
	} {
		if nested, ok := fields[key]; ok {
			merge(usageFragment(nested))
		}
	}
	if nested, ok := fields["tokens"]; ok {
		merge(usageFromTokens(nested), false)
	}
	return usage, inclusive
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
		if read := firstIntField(fields,
			"cached_tokens", "cachedTokens", "cached_input_tokens", "cachedInputTokens",
			"cache_read_input_tokens", "cacheReadInputTokens"); read > 0 {
			usage.CachedInputTokens = read
			inclusive = true
		}
	}
	usage.CachedWriteTokens = firstIntField(fields,
		"cache_creation_input_tokens", "cacheCreationInputTokens",
		"cached_write_tokens", "cachedWriteTokens", "cache_write_tokens", "cacheWriteTokens")
	usage.ReasoningOutputTokens = firstIntField(fields, "reasoning_output_tokens", "reasoningOutputTokens", "reasoning_tokens", "reasoningTokens")
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
	confirmedDisjoint := usage.CachedWriteTokens > 0 ||
		(usage.TotalTokens > 0 && usage.TotalTokens == usage.ComponentTotal())
	confirmedInclusive := usage.TotalTokens > 0 && usage.TotalTokens == usage.InputTokens+usage.OutputTokens
	if read > 0 && read <= usage.InputTokens && !confirmedDisjoint &&
		(confirmedInclusive || (usage.TotalTokens == 0 && inclusiveVocabulary)) {
		usage.InputTokens -= read
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.ComponentTotal()
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
		ReasoningOutputTokens: firstIntField(fields, "reasoning", "reasoning_tokens", "reasoningTokens"),
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
