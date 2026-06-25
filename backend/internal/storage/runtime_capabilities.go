package storage

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NormalizeRuntimeCapabilities(caps *RuntimeCapabilities) *RuntimeCapabilities {
	if caps == nil || !caps.NativeGoal {
		return nil
	}
	out := *caps
	return &out
}

func MarshalRuntimeCapabilities(caps *RuntimeCapabilities) (string, error) {
	caps = NormalizeRuntimeCapabilities(caps)
	if caps == nil {
		return "{}", nil
	}
	data, err := json.Marshal(caps)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UnmarshalRuntimeCapabilities(raw string) (*RuntimeCapabilities, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return nil, nil
	}
	var caps RuntimeCapabilities
	if err := json.Unmarshal([]byte(raw), &caps); err != nil {
		return nil, fmt.Errorf("runtime capabilities: %w", err)
	}
	return NormalizeRuntimeCapabilities(&caps), nil
}
