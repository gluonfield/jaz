package acp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/processenv"
)

func antigravityModelsCommand(ctx context.Context, env map[string]string) (*exec.Cmd, error) {
	path, err := executableInPath("agy", env["PATH"])
	if err != nil {
		path, err = ResolveExecutable("agy")
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, path, "models")
	if len(env) > 0 {
		cmd.Env = processenv.List(env)
	}
	return cmd, nil
}

func antigravityModels(ctx context.Context, env map[string]string) (AgentModelCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd, err := antigravityModelsCommand(ctx, env)
	if err != nil {
		return AgentModelCatalog{}, err
	}
	out, err := cmd.Output()
	if err != nil {
		return AgentModelCatalog{}, fmt.Errorf("list Antigravity models: %w", err)
	}
	catalog := AgentModelCatalog{Models: []AgentModel{}}
	for _, line := range strings.Split(string(out), "\n") {
		id, label, _ := strings.Cut(strings.TrimSpace(line), "\t")
		id, label = strings.TrimSpace(id), strings.TrimSpace(label)
		if id == "" {
			continue
		}
		if label == "" {
			label = id
		}
		catalog.Models = append(catalog.Models, AgentModel{
			Value: id, Label: label,
			Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent},
		})
	}
	return catalog, nil
}
