package sessionevents

type AgentSession struct {
	ConfigOptions []AgentConfigOption `json:"config_options"`
	Commands      []AgentCommand      `json:"commands"`
	Auth          *AgentAuthIdentity  `json:"auth,omitempty"`
	Notices       []string            `json:"notices,omitempty"`
}

type AgentConfigOption struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description,omitempty"`
	Category         string             `json:"category,omitempty"`
	CurrentValue     string             `json:"current_value"`
	UserValue        *string            `json:"user_value,omitempty"`
	RecommendedValue string             `json:"recommended_value,omitempty"`
	Options          []AgentConfigValue `json:"options"`
}

type AgentConfigValue struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
}

type AgentCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputHint   string `json:"input_hint,omitempty"`
}

type AgentAuthIdentity struct {
	Kind    string            `json:"kind"`
	Label   string            `json:"label"`
	Detail  string            `json:"detail,omitempty"`
	Account *AgentAuthAccount `json:"account,omitempty"`
}

type AgentAuthAccount struct {
	Email        string `json:"email,omitempty"`
	Organization string `json:"organization,omitempty"`
	Plan         string `json:"plan,omitempty"`
}

type AgentTask struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Type             string          `json:"type,omitempty"`
	State            string          `json:"state"`
	Description      string          `json:"description,omitempty"`
	Summary          string          `json:"summary,omitempty"`
	LastToolName     string          `json:"last_tool_name,omitempty"`
	OutputFilePath   string          `json:"output_file_path,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	CanStop          bool            `json:"can_stop"`
	ConnectionLost   bool            `json:"connection_lost,omitempty"`
	ShowInTranscript bool            `json:"show_in_transcript"`
	Usage            *AgentTaskUsage `json:"usage,omitempty"`
}

type AgentTaskUsage struct {
	TotalTokens int64 `json:"total_tokens"`
	ToolUses    int64 `json:"tool_uses"`
	DurationMs  int64 `json:"duration_ms"`
}
