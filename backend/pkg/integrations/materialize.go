package integrations

import (
	"context"
	"encoding/json"
)

type SourceProjector interface {
	SourceTargets(context.Context, MaterializeRequest) ([]SourceTarget, error)
	ProjectSource(context.Context, SourceProjectionRequest) (Artifact, error)
}

type MaterializeRequest struct {
	Record Record
}

type SourceProjectionRequest struct {
	Target  SourceTarget
	Records []Record
}

type SourceTarget struct {
	Provider    string   `json:"provider,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	PathHint    string   `json:"path_hint,omitempty"`
	MediaType   string   `json:"media_type,omitempty"`
	ContactRefs []string `json:"contact_refs,omitempty"`
}

type Artifact struct {
	Provider  string          `json:"provider,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	PathHint  string          `json:"path_hint,omitempty"`
	MediaType string          `json:"media_type"`
	Body      []byte          `json:"body"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}
