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
	if usageEmpty(usage) {
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
	if usageEmpty(usage) {
		return
	}
	_ = store.AddUsage(job.ID, usage)
}

func usageFromRaw(raw json.RawMessage) storage.Usage {
	if len(raw) == 0 {
		return storage.Usage{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return storage.Usage{}
	}
	usage := usageFromFields(fields)
	for _, key := range []string{"usage", "tokenUsage", "token_usage"} {
		if nested, ok := fields[key]; ok {
			usage = mergeUsageSnapshot(usage, usageFromRaw(nested))
		}
	}
	if nested, ok := fields["tokens"]; ok {
		usage = mergeUsageSnapshot(usage, usageFromTokens(nested))
	}
	for _, key := range []string{"_meta", "meta", "metadata"} {
		if nested, ok := fields[key]; ok {
			usage = mergeUsageSnapshot(usage, usageFromRaw(nested))
		}
	}
	return usage
}

func usageFromFields(fields map[string]json.RawMessage) storage.Usage {
	var usage storage.Usage
	usage.InputTokens = firstIntField(fields, "input_tokens", "inputTokens", "input", "prompt_tokens", "promptTokens")
	usage.OutputTokens = firstIntField(fields, "output_tokens", "outputTokens", "output", "completion_tokens", "completionTokens")
	usage.CachedInputTokens = firstIntField(fields,
		"cached_input_tokens", "cachedInputTokens", "cached_tokens", "cachedTokens",
		"cache_read_input_tokens", "cacheReadInputTokens", "cache_read_tokens", "cacheReadTokens")
	usage.ReasoningOutputTokens = firstIntField(fields, "reasoning_output_tokens", "reasoningOutputTokens", "reasoning_tokens", "reasoningTokens")
	usage.TotalTokens = firstIntField(fields, "total_tokens", "totalTokens")
	for _, key := range []string{"prompt_tokens_details", "promptTokensDetails", "input_tokens_details", "inputTokensDetails"} {
		if nested, ok := fields[key]; ok {
			usage = mergeUsageSnapshot(usage, usageFromRaw(nested))
		}
	}
	for _, key := range []string{"completion_tokens_details", "completionTokensDetails", "output_tokens_details", "outputTokensDetails"} {
		if nested, ok := fields[key]; ok {
			usage = mergeUsageSnapshot(usage, usageFromRaw(nested))
		}
	}
	return usage
}

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
		}
	}
	return usage
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
	if next.OutputTokens > current.OutputTokens {
		current.OutputTokens = next.OutputTokens
	}
	if next.ReasoningOutputTokens > current.ReasoningOutputTokens {
		current.ReasoningOutputTokens = next.ReasoningOutputTokens
	}
	if next.TotalTokens > current.TotalTokens {
		current.TotalTokens = next.TotalTokens
	}
	if derivedTotal := current.InputTokens + current.OutputTokens; derivedTotal > current.TotalTokens {
		current.TotalTokens = derivedTotal
	}
	return current
}

func usageEmpty(usage storage.Usage) bool {
	return usage.InputTokens == 0 &&
		usage.CachedInputTokens == 0 &&
		usage.OutputTokens == 0 &&
		usage.ReasoningOutputTokens == 0 &&
		usage.TotalTokens == 0
}
