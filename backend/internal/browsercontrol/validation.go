package browsercontrol

import (
	"errors"
	"fmt"
	"strings"
)

const (
	browserURLLimit       = 8192
	browserQueryLimit     = 2000
	browserKeyLimit       = 100
	browserFormValueLimit = 100000
	browserScrollLimit    = 1000000
	browserWaitLimit      = 60000
)

func NormalizeActionInput(input ActionInput) (ActionInput, error) {
	var err error
	if input.Ref != "" || input.Action == ActionClick || input.Action == ActionHover || input.Action == ActionDrag || input.Action == ActionFormInput {
		input.Ref, err = exactRef(input.Ref)
		if err != nil {
			return ActionInput{}, err
		}
	}
	switch input.Action {
	case ActionStatus, ActionTabs, ActionState, ActionAXState, ActionScreenshot, ActionClick, ActionHover:
	case ActionClaimTab:
		input.TabID, err = requiredText(input.TabID, "tab_id", browserTabIDLimit)
	case ActionNavigate:
		input.URL, err = requiredText(input.URL, "url", browserURLLimit)
	case ActionFind:
		input.Text, err = requiredText(input.Text, "query", browserQueryLimit)
	case ActionDrag:
		input.Text, err = exactRef(input.Text)
	case ActionFormInput:
		switch value := input.Value.(type) {
		case string:
			if len(value) > browserFormValueLimit {
				err = errors.New("form value is too long")
			}
		case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		case nil:
			err = errors.New("value is required")
		default:
			err = errors.New("form value must be a string, number or boolean")
		}
	case ActionPress:
		input.Key, err = requiredText(input.Key, "key", browserKeyLimit)
	case ActionScroll:
		input.Text = strings.ToLower(strings.TrimSpace(input.Text))
		if input.Text == "" {
			input.Text = "down"
		}
		switch input.Text {
		case "up", "down", "left", "right":
		default:
			return ActionInput{}, fmt.Errorf("unsupported scroll direction %q", input.Text)
		}
		if input.Amount < 0 {
			err = errors.New("amount must be non-negative")
		} else if input.Amount > browserScrollLimit {
			err = fmt.Errorf("amount must not exceed %d", browserScrollLimit)
		}
	case ActionWait:
		input.Text = strings.TrimSpace(input.Text)
		if input.Amount < 0 {
			err = errors.New("timeout_ms must be non-negative")
		} else if input.Amount > browserWaitLimit {
			err = fmt.Errorf("timeout_ms must not exceed %d", browserWaitLimit)
		} else if len(input.Text) > browserQueryLimit {
			err = errors.New("wait text is too long")
		}
	case ActionScript:
		if len(input.Text) > 100000 {
			err = errors.New("browser code exceeds 100000 bytes")
		}
	default:
		err = UnsupportedActionError{Action: input.Action}
	}
	return input, err
}

func requiredText(value, name string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if len(value) > limit {
		return "", fmt.Errorf("%s is too long", name)
	}
	return value, nil
}

func exactRef(value string) (string, error) {
	return requiredText(strings.TrimPrefix(strings.TrimSpace(value), "ref="), "ref", stateRefLimit)
}
