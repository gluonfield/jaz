package browserworker

const (
	ActionStatus     = "status"
	ActionTabs       = "tabs"
	ActionNavigate   = "navigate"
	ActionSnapshot   = "snapshot"
	ActionState      = "state"
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
	ActionNavigate,
	ActionSnapshot,
	ActionState,
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
