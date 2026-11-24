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
	if err := ctx.Err(); err != nil {
		return AgentModelCatalog{}, err
	}
	if catalog, ok := cachedAntigravityCatalog(env); ok {
		return catalog, nil
	}
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
	return antigravityCatalog(string(out)), nil
}

// antigravityThoughtLevels are the levels agy accepts, lowest first.
var antigravityThoughtLevels = []string{"low", "medium", "high", "xhigh", "max"}

// antigravityCatalog reads agy's "id<TAB>label" listing. agy lists a model once
// for each thought level it offers, and its session options take the model and
// the level separately, so those entries become one model with its levels as
// efforts and their ids as aliases.
func antigravityCatalog(listing string) AgentModelCatalog {
	catalog := AgentModelCatalog{Models: []AgentModel{}}
	listed := map[string]int{}
	for _, line := range strings.Split(listing, "\n") {
		id, label, _ := strings.Cut(strings.TrimSpace(line), "\t")
		id, label = strings.TrimSpace(id), strings.TrimSpace(label)
		if id == "" {
			continue
		}
		if label == "" {
			label = id
		}
		model, name, level := cutAntigravityThoughtLevel(id, label)
		i, seen := listed[model]
		if !seen {
			i = len(catalog.Models)
			listed[model] = i
			catalog.Models = append(catalog.Models, AgentModel{
				Value: model, Label: name,
				Reasoning: ReasoningCapabilities{Status: modelcatalog.ReasoningUnavailable, Scope: ReasoningScopeAgent},
			})
		}
		if level == "" {
			continue
		}
		reasoning := &catalog.Models[i].Reasoning
		reasoning.Status = modelcatalog.ReasoningReady
		reasoning.Efforts = append(reasoning.Efforts, level)
		catalog.Models[i].Aliases = append(catalog.Models[i].Aliases, id)
	}
	for i := range catalog.Models {
		slices.SortFunc(catalog.Models[i].Reasoning.Efforts, func(a, b string) int {
			return slices.Index(antigravityThoughtLevels, a) - slices.Index(antigravityThoughtLevels, b)
		})
	}
	return catalog
}

// cutAntigravityThoughtLevel splits an entry such as "gemini-4-neon-xhigh",
// labelled "Gemini 4 Neon (XHigh)", into its model, the model's name and the
// level. An entry whose id and label do not name the same level is a model
// without one.
func cutAntigravityThoughtLevel(id, label string) (model, name, level string) {
	open := strings.LastIndex(label, " (")
	if open < 0 || !strings.HasSuffix(label, ")") {
		return id, label, ""
	}
	level = strings.ToLower(label[open+2 : len(label)-1])
	model, ok := strings.CutSuffix(id, "-"+level)
	if !ok || model == "" || !slices.Contains(antigravityThoughtLevels, level) {
		return id, label, ""
	}
	return model, label[:open], level
}
