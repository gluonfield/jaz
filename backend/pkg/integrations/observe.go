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

type ObserveMode string

const (
	ObserveModeIncremental ObserveMode = "incremental"
	ObserveModeBackfill    ObserveMode = "initial_backfill"
	ObserveModeRepair      ObserveMode = "repair"
)

type Cursor struct {
	Kind  string          `json:"kind,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

func (c Cursor) Empty() bool {
	return c.Kind == "" && len(c.Value) == 0
}

type ObserveRequest struct {
	Connection Connection
	Client     *http.Client
	Cursor     Cursor
	Mode       ObserveMode
	Since      time.Time
}

type ObserveResult struct {
	Records []Record `json:"records,omitempty"`
	Cursor  Cursor   `json:"cursor,omitempty"`
}
