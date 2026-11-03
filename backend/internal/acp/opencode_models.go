package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/modelcatalog"
)

// opencodeNativeModels discovers the model catalog for the OpenCode accounts
// provider the same way Kimi does: a short-lived ACP session whose session/new
// response carries the agent's configOptions. Unlike Kimi, OpenCode returns the
// full model list in a single option, so no per-model selection is needed.
func (m *Manager) opencodeNativeModels(ctx context.Context, cfg AgentConfig) (catalog AgentModelCatalog, err error) {
	lifetime, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return catalog, err
	}
	cwd := filepath.Join(firstNonEmpty(m.cfg.Root, filepath.Join(os.TempDir(), "jaz")), "acp", "model-discovery")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		return catalog, err
	}
	ac, err := m.connectWithHandler(ctx, AgentOpenCode, cfg, cwd, "", "", nil, jsonrpc.HandlerFunc(func(_ context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		return nil, jsonrpc.MethodNotFound(req.Method)
	}))
	if err != nil {
		return catalog, err
	}
	defer ac.close()
	raw, err := ac.peer.Call(lifetime, acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{Cwd: cwd, MCPServers: []acpschema.MCPServer{}})
	if err != nil {
		return catalog, err
	}
	state := parseSessionConfigOptions(raw)
	catalog = AgentModelCatalog{Models: []AgentModel{}, DefaultModel: state.currentModel}
	var response struct {
		Options []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
			Models   []struct {
				Value       string `json:"value"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"configOptions"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return catalog, fmt.Errorf("decode OpenCode model discovery: %w", err)
	}
	for _, option := range response.Options {
		if option.Category != "model" && option.ID != sessionConfigModel {
			continue
		}
		for _, choice := range option.Models {
			catalog.Models = append(catalog.Models, AgentModel{
				Value:       choice.Value,
				Label:       choice.Name,
				Description: choice.Description,
				Reasoning:   ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent},
			})
		}
	}
	if len(catalog.Models) == 0 {
		return catalog, fmt.Errorf("OpenCode model discovery returned no models")
	}
	return catalog, nil
}
