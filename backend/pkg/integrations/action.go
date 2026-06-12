package integrations

import (
	"context"
	"encoding/json"
	"net/http"
)

type ActionProvider interface {
	Actions(context.Context, Connection) ([]ActionSpec, error)
	ExecuteAction(context.Context, ActionRequest) (ActionResult, error)
}

type ActionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ReadOnly    bool            `json:"read_only"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type ActionRequest struct {
	Action     string
	Connection Connection
	Client     *http.Client
	Arguments  json.RawMessage
}

type ActionResult struct {
	Text string          `json:"text,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}
