package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func (m *Manager) kimiModels(ctx context.Context, cfg AgentConfig) (catalog AgentModelCatalog, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cwd := filepath.Join(firstNonEmpty(m.cfg.Root, filepath.Join(os.TempDir(), "jaz")), "acp", "model-discovery")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		return catalog, err
	}
	ac, err := m.connectWithHandler(ctx, AgentKimi, cfg, cwd, "", "", nil, jsonrpc.HandlerFunc(func(_ context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		return nil, jsonrpc.MethodNotFound(req.Method)
	}))
	if err != nil {
		return catalog, err
	}
	defer ac.close()
	var initialized acpschema.InitializeResponse
	if err := json.Unmarshal(ac.initRaw, &initialized); err != nil {
		return catalog, err
	}
	capabilities := initialized.AgentCapabilities
	if capabilities == nil || capabilities.SessionCapabilities == nil || capabilities.SessionCapabilities.Delete == nil {
		return catalog, fmt.Errorf("Kimi model discovery requires session/delete support")
	}
	raw, err := ac.peer.Call(ctx, acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{Cwd: cwd, MCPServers: []acpschema.MCPServer{}})
	if err != nil {
		return catalog, err
	}
	var session acpschema.NewSessionResponse
	if err := json.Unmarshal(raw, &session); err != nil || session.SessionID == "" {
		return catalog, fmt.Errorf("Kimi model discovery returned an invalid session")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_, deleteErr := ac.peer.Call(cleanup, "session/delete", map[string]acpschema.SessionID{"sessionId": session.SessionID})
		if deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("delete Kimi model discovery session: %w", deleteErr))
		}
	}()
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
		return catalog, err
	}
	state := parseSessionConfigOptions(raw)
	catalog = AgentModelCatalog{Models: []AgentModel{}, DefaultModel: state.currentModel}
	for _, option := range response.Options {
		if option.Category != "model" && option.ID != sessionConfigModel {
			continue
		}
		for _, choice := range option.Models {
			if state.currentModel != choice.Value {
				raw, err = setSessionConfigOption(ctx, ac.peer, session.SessionID, option.ID, choice.Value)
				if err != nil {
					return catalog, err
				}
				state = parseSessionConfigOptions(raw)
			}
			if state.currentModel != choice.Value {
				return catalog, fmt.Errorf("Kimi did not select model %q", choice.Value)
			}
			model := AgentModel{Value: choice.Value, Label: choice.Name, Description: choice.Description,
				Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent}}
			if len(state.effortOptions) > 0 {
				// Kimi can append the previous model's current effort until a new one is selected.
				raw, err = setSessionConfigOption(ctx, ac.peer, session.SessionID, state.effortConfigID, state.effortOptions[0])
				if err != nil {
					return catalog, err
				}
				state = parseSessionConfigOptions(raw)
				model.Reasoning.Status = modelcatalog.ReasoningReady
				model.Reasoning.Efforts = state.effortOptions
				model.Reasoning.Mandatory = !slices.Contains(state.effortOptions, "off")
			}
			catalog.Models = append(catalog.Models, model)
		}
	}
	return catalog, nil
}
