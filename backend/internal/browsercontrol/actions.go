package browsercontrol

import (
	"encoding/base64"
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
	ActionNavigate   = "navigate"
	ActionState      = "state"
	ActionAXState    = "ax_state"
	ActionFind       = "find"
	ActionScreenshot = "screenshot"
	ActionClick      = "click"
	ActionHover      = "hover"
	ActionDrag       = "drag"
	ActionScript     = "script"
	ActionFormInput  = "form_input"
	ActionPress      = "press"
	ActionScroll     = "scroll"
	ActionWait       = "wait"
)

type UnsupportedActionError struct {
	Action string
}

func (e UnsupportedActionError) Error() string {
	return fmt.Sprintf("unsupported browser action %q", e.Action)
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
