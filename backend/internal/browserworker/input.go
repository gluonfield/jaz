package browserworker

import (
	"errors"
	"strings"
)

func scrollDelta(direction string, amount int) (int, int) {
	if amount == 0 {
		amount = defaultScrollAmount
	}
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "up":
		return -abs(amount), 0
	case "left":
		return 0, -abs(amount)
	case "right":
		return 0, abs(amount)
	default:
		return amount, 0
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func keyEvent(key string) (map[string]any, error) {
	if key == "" {
		return nil, errors.New("key is required")
	}
	switch strings.ToLower(key) {
	case "enter", "return":
		return map[string]any{"key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "nativeVirtualKeyCode": 13}, nil
	case "tab":
		return map[string]any{"key": "Tab", "code": "Tab", "windowsVirtualKeyCode": 9, "nativeVirtualKeyCode": 9}, nil
	case "escape", "esc":
		return map[string]any{"key": "Escape", "code": "Escape", "windowsVirtualKeyCode": 27, "nativeVirtualKeyCode": 27}, nil
	case "backspace":
		return map[string]any{"key": "Backspace", "code": "Backspace", "windowsVirtualKeyCode": 8, "nativeVirtualKeyCode": 8}, nil
	case "arrowdown", "down":
		return map[string]any{"key": "ArrowDown", "code": "ArrowDown", "windowsVirtualKeyCode": 40, "nativeVirtualKeyCode": 40}, nil
	case "arrowup", "up":
		return map[string]any{"key": "ArrowUp", "code": "ArrowUp", "windowsVirtualKeyCode": 38, "nativeVirtualKeyCode": 38}, nil
	case "arrowleft", "left":
		return map[string]any{"key": "ArrowLeft", "code": "ArrowLeft", "windowsVirtualKeyCode": 37, "nativeVirtualKeyCode": 37}, nil
	case "arrowright", "right":
		return map[string]any{"key": "ArrowRight", "code": "ArrowRight", "windowsVirtualKeyCode": 39, "nativeVirtualKeyCode": 39}, nil
	default:
		if len([]rune(key)) == 1 {
			return map[string]any{"key": key, "text": key}, nil
		}
		return map[string]any{"key": key, "code": key}, nil
	}
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
