package browserworker

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ActionStatus     = "status"
	ActionTabs       = "tabs"
	ActionAdoptTab   = "adopt_active_tab"
	ActionNavigate   = "navigate"
	ActionSnapshot   = "snapshot"
	ActionState      = "state"
	ActionExtract    = "extract"
	ActionScreenshot = "screenshot"
	ActionClick      = "click"
	ActionHover      = "hover"
	ActionType       = "type"
	ActionFill       = "fill"
	ActionSelect     = "select"
	ActionPress      = "press"
	ActionScroll     = "scroll"
	ActionWait       = "wait"
	ActionPDF        = "pdf"
)

var supportedExtensionActions = []string{
	ActionStatus,
	ActionTabs,
	ActionAdoptTab,
	ActionNavigate,
	ActionSnapshot,
	ActionState,
	ActionExtract,
	ActionScreenshot,
	ActionClick,
	ActionHover,
	ActionType,
	ActionFill,
	ActionSelect,
	ActionPress,
	ActionScroll,
	ActionWait,
}

func SupportedExtensionActions() []string {
	return append([]string(nil), supportedExtensionActions...)
}

var optionalExtensionActions = map[string]bool{
	ActionAdoptTab: true,
	ActionExtract:  true,
}

func requiredExtensionActions() []string {
	actions := SupportedExtensionActions()
	required := make([]string, 0, len(actions))
	for _, action := range actions {
		if !optionalExtensionActions[action] {
			required = append(required, action)
		}
	}
	return required
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
