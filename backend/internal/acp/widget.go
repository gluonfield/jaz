package acp

import (
	"encoding/json"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

// ClientMethodWidgetPublish is a jaz ACP extension method (underscore prefix
// per the ACP custom-method convention). Its availability is advertised via
// ClientCapabilities._meta["jaz.dev/widget"] during initialize.
const ClientMethodWidgetPublish = "_jaz.dev/widget/publish"

type WidgetPublishRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title,omitempty"`
	SizeHint  string `json:"sizeHint,omitempty"`
	HTML      string `json:"html,omitempty"`
}

type WidgetPublishResult struct {
	WidgetID string `json:"widgetId"`
	Title    string `json:"title"`
	Version  int    `json:"version"`
	SizeHint string `json:"sizeHint,omitempty"`
	// Non-fatal quality problems; the agent should fix and republish in-run.
	Warnings []string `json:"warnings,omitempty"`
}

func (m *Manager) widgetPublish(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req WidgetPublishRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid "+ClientMethodWidgetPublish, map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(req.SessionID)
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	// The publisher works in jaz session ids; the agent only knows its own.
	req.SessionID = job.ID
	result, err := m.PublishWidget(req)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	return jsonrpc.EncodeResult(result)
}
