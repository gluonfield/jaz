package browserworker

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
