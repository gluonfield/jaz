package connections

const (
	AgentPathKindMemoryPage   = "memory_page"
	AgentPathKindMemoryPrefix = "memory_prefix"
)

type AgentPath struct {
	Path        string `json:"path"`
	Kind        string `json:"kind,omitempty"`
	Explanation string `json:"explanation"`
}

type AgentConnection struct {
	ProviderID    string      `json:"provider_id"`
	ProviderName  string      `json:"provider_name"`
	Account       string      `json:"account"`
	RelevantPaths []AgentPath `json:"relevant_paths,omitempty"`
}
