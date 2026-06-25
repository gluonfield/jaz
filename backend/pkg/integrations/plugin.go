package integrations

type AuthKind string

const (
	AuthKindOAuth        AuthKind = "oauth"
	AuthKindSession      AuthKind = "session"
	AuthKindBridge       AuthKind = "bridge"
	AuthKindRemoteMCP    AuthKind = "remote_mcp"
	AuthKindBrowserLocal AuthKind = "browser_local"
)

type Capability string

const (
	CapabilitySync        Capability = "sync"
	CapabilityAct         Capability = "act"
	CapabilityMaterialize Capability = "materialize"
	CapabilityMCP         Capability = "mcp"
	CapabilityBrowser     Capability = "browser"
)

type ActionRisk string

const (
	ActionRiskRead      ActionRisk = "read"
	ActionRiskDraft     ActionRisk = "draft"
	ActionRiskWrite     ActionRisk = "write"
	ActionRiskBulkWrite ActionRisk = "bulk_write"
	ActionRiskDelete    ActionRisk = "delete"
)

type Plugin struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	Provider        Provider       `json:"provider"`
	Auth            []AuthOption   `json:"auth"`
	Capabilities    []Capability   `json:"capabilities"`
	MultiAccount    bool           `json:"multi_account"`
	SourceLanes     []string       `json:"source_lanes,omitempty"`
	Tools           []PluginTool   `json:"tools,omitempty"`
	Skills          []PluginSkill  `json:"skills,omitempty"`
	RemoteMCP       *RemoteMCP     `json:"remote_mcp,omitempty"`
	ConnectionNotes []string       `json:"connection_notes,omitempty"`
	Implementation  Implementation `json:"implementation"`
}

type Implementation struct {
	Status string `json:"status"`
	Owner  string `json:"owner"`
}

type AuthOption struct {
	Kind        AuthKind `json:"kind"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

type RemoteMCP struct {
	URL          string   `json:"url"`
	Status       string   `json:"status"`
	Requires     []string `json:"requires,omitempty"`
	OAuthSecrets bool     `json:"oauth_secrets"`
}

type PluginTool struct {
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Capability     Capability `json:"capability"`
	Risk           ActionRisk `json:"risk"`
	RequiredScopes []string   `json:"required_scopes,omitempty"`
}

type PluginSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}
