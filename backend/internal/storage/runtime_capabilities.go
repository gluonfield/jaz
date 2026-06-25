package storage

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NormalizeRuntimeCapabilities(caps *RuntimeCapabilities) *RuntimeCapabilities {
	if caps == nil || (!caps.NativeGoal && !caps.NativeGoalNegotiable) {
		return nil
	}
	out := *caps
	if out.NativeGoal {
		out.NativeGoalNegotiable = false
	}
	return &out
}

func NormalizePersistedRuntimeCapabilities(caps *RuntimeCapabilities) *RuntimeCapabilities {
	if caps == nil || !caps.NativeGoal {
		return nil
	}
	return &RuntimeCapabilities{NativeGoal: true}
}

func MarshalRuntimeCapabilities(caps *RuntimeCapabilities) (string, error) {
	caps = NormalizePersistedRuntimeCapabilities(caps)
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
	return NormalizePersistedRuntimeCapabilities(&caps), nil
}
