package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Observer interface {
	Observe(context.Context, ObserveRequest) (ObserveResult, error)
}

type Cursor struct {
	Kind  string          `json:"kind,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

type ObserveRequest struct {
	Connection Connection
	Client     *http.Client
	Cursor     Cursor
	Since      time.Time
}

type ObserveResult struct {
	Records []Record `json:"records,omitempty"`
	Cursor  Cursor   `json:"cursor,omitempty"`
}
