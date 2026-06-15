package openai

import (
	"encoding/json"

	oa "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/respjson"
)

func cachedInputTokens(usage oa.CompletionUsage) int64 {
	if usage.PromptTokensDetails.CachedTokens > 0 {
		return usage.PromptTokensDetails.CachedTokens
	}
	for _, key := range []string{"cache_read_input_tokens", "cached_tokens"} {
		if value, ok := extraInt64(usage.JSON.ExtraFields, key); ok {
			return value
		}
	}
	return 0
}

func cachedWriteTokens(usage oa.CompletionUsage) int64 {
	for _, fields := range []map[string]respjson.Field{
		usage.PromptTokensDetails.JSON.ExtraFields,
		usage.JSON.ExtraFields,
	} {
		for _, key := range []string{"cache_write_tokens", "cached_write_tokens", "cache_creation_input_tokens"} {
			if value, ok := extraInt64(fields, key); ok {
				return value
			}
		}
	}
	return 0
}

func extraInt64(extras map[string]respjson.Field, key string) (int64, bool) {
	field, ok := extras[key]
	if !ok {
		return 0, false
	}
	raw := field.Raw()
	if raw == "" || raw == "null" {
		return 0, false
	}
	var value int64
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return 0, false
	}
	return value, true
}
