package acp

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/processenv"
)

func antigravityModelsCommand(ctx context.Context, env map[string]string) (*exec.Cmd, error) {
	path, err := antigravityExecutable(env["PATH"])
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, "models")
	if len(env) > 0 {
		cmd.Env = processenv.List(env)
	}
	return cmd, nil
}

func antigravityExecutable(dirs string) (string, error) {
	for _, name := range []string{"antigravity", "agy"} {
		if path, err := executableInPath(name, dirs); err == nil {
			return path, nil
		}
	}
	if path, err := ResolveExecutable("antigravity"); err == nil {
		return path, nil
	}
	return ResolveExecutable("agy")
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
	levelled := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		id, label, _ := strings.Cut(strings.TrimSpace(line), "\t")
		id, label = strings.TrimSpace(id), strings.TrimSpace(label)
		if id == "" {
			continue
		}
		if label == "" {
			label = id
		}
		model, name, level, ok := cutAntigravityThoughtLevel(id, label)
		if !ok {
			catalog.Models = append(catalog.Models, AgentModel{
				Value: id, Label: label,
				Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent},
			})
			continue
		}
		i, seen := levelled[model]
		if !seen {
			i = len(catalog.Models)
			levelled[model] = i
			catalog.Models = append(catalog.Models, AgentModel{
				Value: model, Label: name,
				Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningReady, Scope: ReasoningScopeAgent},
			})
		}
		entry := &catalog.Models[i]
		entry.Aliases = append(entry.Aliases, id)
		entry.Reasoning.Efforts = append(entry.Reasoning.Efforts, level)
	}
	for _, model := range catalog.Models {
		slices.SortFunc(model.Reasoning.Efforts, func(a, b string) int {
			return slices.Index(antigravityThoughtLevels, a) - slices.Index(antigravityThoughtLevels, b)
		})
	}
	return catalog, nil
}

// antigravityThoughtLevels are the levels agy accepts, lowest first.
var antigravityThoughtLevels = []string{"low", "medium", "high", "xhigh", "max"}

// cutAntigravityThoughtLevel splits an entry agy lists once per thought level,
// such as "gemini-4-neon-xhigh" labelled "Gemini 4 Neon (XHigh)", into the
// model and level its session options take. The id and the label must name the
// same level.
func cutAntigravityThoughtLevel(id, label string) (model, name, level string, ok bool) {
	open := strings.LastIndex(label, " (")
	if open < 0 || !strings.HasSuffix(label, ")") {
		return "", "", "", false
	}
	level = strings.ToLower(label[open+2 : len(label)-1])
	model, ok = strings.CutSuffix(id, "-"+level)
	if !ok || model == "" || !slices.Contains(antigravityThoughtLevels, level) {
		return "", "", "", false
	}
	return model, label[:open], level, true
}
