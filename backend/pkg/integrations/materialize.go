package integrations

import (
	"context"
	"encoding/json"
)

type Materializer interface {
	Materialize(context.Context, MaterializeRequest) ([]Artifact, error)
}

type MaterializeRequest struct {
	Connection Connection
	Record     Record
}

type Artifact struct {
	Kind      string          `json:"kind,omitempty"`
	PathHint  string          `json:"path_hint,omitempty"`
	MediaType string          `json:"media_type"`
	Body      []byte          `json:"body"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}
