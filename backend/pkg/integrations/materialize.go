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
	Provider    string    `json:"provider,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	PathHint    string    `json:"path_hint,omitempty"`
	MediaType   string    `json:"media_type,omitempty"`
	Key         SourceKey `json:"key,omitempty"`
	Replay      Replay    `json:"replay,omitempty"`
	ContactRefs []string  `json:"contact_refs,omitempty"`
}

func (t SourceTarget) MarshalJSON() ([]byte, error) {
	var out struct {
		Provider    string     `json:"provider,omitempty"`
		Kind        string     `json:"kind,omitempty"`
		PathHint    string     `json:"path_hint,omitempty"`
		MediaType   string     `json:"media_type,omitempty"`
		Key         *SourceKey `json:"key,omitempty"`
		Replay      *Replay    `json:"replay,omitempty"`
		ContactRefs []string   `json:"contact_refs,omitempty"`
	}
	out.Provider = t.Provider
	out.Kind = t.Kind
	out.PathHint = t.PathHint
	out.MediaType = t.MediaType
	out.ContactRefs = t.ContactRefs
	if !t.Key.IsZero() {
		key := t.Key
		out.Key = &key
	}
	if !t.Replay.IsZero() {
		replay := t.Replay
		out.Replay = &replay
	}
	return json.Marshal(out)
}

type SourceKey struct {
	Entity string `json:"entity,omitempty"`
	Day    string `json:"day,omitempty"`
}

func (k SourceKey) IsZero() bool {
	return k.Entity == "" && k.Day == ""
}

type Replay struct {
	Account string        `json:"account,omitempty"`
	Scopes  []ReplayScope `json:"scopes,omitempty"`
}

func (r Replay) IsZero() bool {
	return r.Account == "" && len(r.Scopes) == 0
}

type ReplayScope struct {
	Domain RecordDomain `json:"domain,omitempty"`
	Day    string       `json:"day,omitempty"`
}

type Artifact struct {
	Provider  string          `json:"provider,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	PathHint  string          `json:"path_hint,omitempty"`
	MediaType string          `json:"media_type"`
	Body      []byte          `json:"body"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}
