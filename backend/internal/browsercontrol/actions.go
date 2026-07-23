package browsercontrol

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	browserImageBase64Limit = 32 << 20
	browserWireReadLimit    = 34 << 20
)

const (
	ActionStatus     = "status"
	ActionTabs       = "tabs"
	ActionClaimTab   = "claim_tab"
	ActionNavigate   = "navigate"
	ActionState      = "state"
	ActionFind       = "find"
	ActionScreenshot = "screenshot"
	ActionClick      = "click"
	ActionFormInput  = "form_input"
	ActionPress      = "press"
	ActionScroll     = "scroll"
	ActionWait       = "wait"
)

var supportedExtensionActions = []string{
	ActionStatus,
	ActionTabs,
	ActionClaimTab,
	ActionNavigate,
	ActionState,
	ActionFind,
	ActionScreenshot,
	ActionClick,
	ActionFormInput,
	ActionPress,
	ActionScroll,
	ActionWait,
}

func SupportedExtensionActions() []string {
	return append([]string(nil), supportedExtensionActions...)
}

type UnsupportedActionError struct {
	Action string
	Hint   string
}

func (e UnsupportedActionError) Error() string {
	action := strings.TrimSpace(e.Action)
	if strings.TrimSpace(e.Hint) != "" {
		return fmt.Sprintf("unsupported browser action %q: %s", action, strings.TrimSpace(e.Hint))
	}
	return fmt.Sprintf("unsupported browser action %q", action)
}

func IsUnsupportedAction(err error, action string) bool {
	var unsupported UnsupportedActionError
	if !errors.As(err, &unsupported) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(unsupported.Action), strings.TrimSpace(action))
}

func decodeImageBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if len(value) > browserImageBase64Limit {
		return nil, fmt.Errorf("browser screenshot exceeds %d encoded bytes", browserImageBase64Limit)
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode browser screenshot: %w", err)
	}
	return data, nil
}
