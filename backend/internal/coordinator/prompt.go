package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/jazagent"
	"github.com/wins/jaz/backend/internal/templates/jazplatform"
	"github.com/wins/jaz/backend/internal/visualize"
)

// PromptFiles are editable personal files. BOTS.md is loaded only by bot chats.
var PromptFiles = []string{"AGENTS.md", "SOUL.md", "BOTS.md"}

type CloudScope struct {
	OrgID   string
	SpaceID string
	UserID  string
	BotID   string
}

type cloudScopeKey struct{}

func WithCloudScope(ctx context.Context, scope CloudScope) context.Context {
	return context.WithValue(ctx, cloudScopeKey{}, scope)
}

func CloudScopeFromContext(ctx context.Context) (CloudScope, bool) {
	scope, ok := ctx.Value(cloudScopeKey{}).(CloudScope)
	return scope, ok
}

func mergeCloudScope(base *CloudScope, ctx context.Context) *CloudScope {
	fromCtx, hasCtx := CloudScopeFromContext(ctx)
	if base == nil && !hasCtx {
		return nil
	}
	var merged CloudScope
	if base != nil {
		merged = *base
	}
	if hasCtx {
		if fromCtx.OrgID != "" {
			merged.OrgID = fromCtx.OrgID
		}
		if fromCtx.SpaceID != "" {
			merged.SpaceID = fromCtx.SpaceID
		}
		if fromCtx.UserID != "" {
			merged.UserID = fromCtx.UserID
		}
		if fromCtx.BotID != "" {
			merged.BotID = fromCtx.BotID
		}
	}
	return &merged
}

func Prompt(root, workspace, memoryRoot, skillsPrompt string) (string, error) {
	return prompt(context.Background(), root, workspace, memoryRoot, skillsPrompt, nil, nil, false, visualize.SurfaceChat, time.Now(), nil, nil)
}

// prompt adds Jaz's native identity; ACP agents keep their provider's identity.
func prompt(ctx context.Context, root, workspace, memoryRoot, skillsPrompt string, connections []connections.AgentConnection, agentNames []string, browserEnabled bool, surface visualize.Surface, now time.Time, inst instructions.Store, cloud *CloudScope) (string, error) {
	if cloud == nil && strings.TrimSpace(workspace) == "" {
		workspace = defaultWorkspace(root)
	}
	agentPrompt, err := jazagent.Render()
	if err != nil {
		return "", err
	}
	platform, err := platformPrompt(ctx, root, workspace, workspace, memoryRoot, skillsPrompt, connections, agentNames, browserEnabled, surface, now, inst, cloud)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(agentPrompt, "\n") + "\n\n" + platform, nil
}

func defaultWorkspace(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return "~/.jaz/workspaces/default"
	}
	return filepath.Join(root, "workspaces", "default")
}

type loadedInstructions struct {
	agents string
	soul   string
	bots   string
	cloud  *jazplatform.CloudInstructions
}

func loadPromptInstructions(ctx context.Context, root string, inst instructions.Store, cloud *CloudScope, isBot bool) (loadedInstructions, error) {
	if inst == nil {
		if cloud != nil {
			return loadedInstructions{}, fmt.Errorf("cloud prompt mode requires an instructions store")
		}
		inst = instructions.NewLocalFSStore(root)
	}
	if cloud == nil {
		agents, err := readInstruction(ctx, inst, instructions.Key{Scope: instructions.ScopeUser, Kind: instructions.KindAgents})
		if err != nil {
			return loadedInstructions{}, err
		}
		soul, err := readInstruction(ctx, inst, instructions.Key{Scope: instructions.ScopeUser, Kind: instructions.KindSoul})
		if err != nil {
			return loadedInstructions{}, err
		}
		var botText string
		if isBot {
			botText, err = readInstruction(ctx, inst, instructions.Key{Scope: instructions.ScopeUser, Kind: instructions.KindBots})
			if err != nil {
				return loadedInstructions{}, err
			}
		}
		return loadedInstructions{agents: agents, soul: soul, bots: botText}, nil
	}
	var orgAgents, spaceAgents string
	var err error
	if cloud.OrgID != "" {
		orgAgents, err = readInstruction(ctx, inst, instructions.Key{
			OrgID:   cloud.OrgID,
			Scope:   instructions.ScopeOrg,
			ScopeID: cloud.OrgID,
			Kind:    instructions.KindAgents,
		})
		if err != nil {
			return loadedInstructions{}, err
		}
	}
	if cloud.SpaceID != "" {
		spaceAgents, err = readInstruction(ctx, inst, instructions.Key{
			OrgID:   cloud.OrgID,
			Scope:   instructions.ScopeSpace,
			ScopeID: cloud.SpaceID,
			Kind:    instructions.KindAgents,
		})
		if err != nil {
			return loadedInstructions{}, err
		}
	}
	agents, err := readInstruction(ctx, inst, instructions.Key{
		OrgID:   cloud.OrgID,
		Scope:   instructions.ScopeUser,
		ScopeID: cloud.UserID,
		Kind:    instructions.KindAgents,
	})
	if err != nil {
		return loadedInstructions{}, err
	}
	soul, err := readInstruction(ctx, inst, instructions.Key{
		OrgID:   cloud.OrgID,
		Scope:   instructions.ScopeUser,
		ScopeID: cloud.UserID,
		Kind:    instructions.KindSoul,
	})
	if err != nil {
		return loadedInstructions{}, err
	}
	var botText string
	if isBot {
		botKey := instructions.Key{OrgID: cloud.OrgID, Scope: instructions.ScopeUser, ScopeID: cloud.UserID, Kind: instructions.KindBots}
		if cloud.BotID != "" {
			botKey = instructions.Key{OrgID: cloud.OrgID, Scope: instructions.ScopeBot, ScopeID: cloud.BotID, Kind: instructions.KindBots}
		}
		botText, err = readInstruction(ctx, inst, botKey)
		if err != nil {
			return loadedInstructions{}, err
		}
	}
	return loadedInstructions{
		agents: agents,
		soul:   soul,
		bots:   botText,
		cloud: &jazplatform.CloudInstructions{
			HasOrg:      cloud.OrgID != "",
			OrgAgents:   orEmpty(orgAgents),
			HasSpace:    cloud.SpaceID != "",
			SpaceAgents: orEmpty(spaceAgents),
		},
	}, nil
}

// platformPrompt selects a role template and loads its scoped inputs.
func platformPrompt(ctx context.Context, root, cwd, workspace, memoryRoot, skillsPrompt string, connections []connections.AgentConnection, agentNames []string, browserEnabled bool, surface visualize.Surface, now time.Time, inst instructions.Store, cloud *CloudScope) (string, error) {
	sourceType := sessioncontext.SourceType(ctx)
	isBot := sourceType == storage.SourceBot || sourceType == storage.SourceBotMember
	loaded, err := loadPromptInstructions(ctx, root, inst, cloud, isBot)
	if err != nil {
		return "", err
	}
	memory, err := memoryData(memoryRoot, now)
	if err != nil {
		return "", err
	}
	surface = visualize.NormalizeSurface(string(surface))
	canvasDir := ""
	if surface == visualize.SurfaceCanvas {
		sessionID := mcpsession.ID(ctx)
		if sessionID == "" {
			return "", fmt.Errorf("canvas prompt needs the session id")
		}
		canvasDir = canvas.Dir(runtimefiles.New(root).Canvases, sessionID)
	}
	data := jazplatform.Data{
		Agents:          orEmpty(loaded.agents),
		AgentNames:      acp.SelectableAgentNames(agentNames),
		Cloud:           loaded.cloud,
		Date:            now.Format("January 2, 2006"),
		Time:            now.Format("15:04:05 MST"),
		Timezone:        now.Format("MST (UTCZ07:00)"),
		Weekday:         now.Format("Monday"),
		Human:           now.Format("Monday, January 2, 2006 at 15:04:05 MST"),
		Cwd:             strings.TrimSpace(cwd),
		Device:          sessioncontext.ClientPlatform(ctx),
		Soul:            orEmpty(loaded.soul),
		BrowserEnabled:  browserEnabled,
		ArtifactSurface: string(surface),
		CanvasDir:       canvasDir,
		Memory:          memory,
		Connections:     connections,
		Skills:          strings.TrimSpace(skillsPrompt),
	}
	if cloud == nil {
		data.RuntimePaths = runtimePaths(root, workspace)
	}
	if isBot {
		return jazplatform.RenderBot(jazplatform.BotData{Data: data, Instructions: orEmpty(loaded.bots)})
	}
	return jazplatform.RenderThread(data)
}

func readInstruction(ctx context.Context, store instructions.Store, key instructions.Key) (string, error) {
	doc, err := store.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(doc.Content), nil
}

func runtimePaths(root, workspace string) jazplatform.RuntimePaths {
	root = displayRoot(root)
	defaultWorkspacePath := strings.TrimSpace(workspace)
	if defaultWorkspacePath == "" {
		defaultWorkspacePath = defaultWorkspace(root)
	}
	return jazplatform.RuntimePaths{
		Root:             root,
		AgentsPath:       filepath.Join(root, "AGENTS.md"),
		SoulPath:         filepath.Join(root, "SOUL.md"),
		SkillsPath:       filepath.Join(root, "skills"),
		SessionsPath:     filepath.Join(root, "sessions"),
		DefaultWorkspace: defaultWorkspacePath,
		WorktreesPath:    filepath.Join(defaultWorkspacePath, ".worktrees"),
	}
}

func displayRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return "~/.jaz"
	}
	return root
}

// ReadPromptFile reads a single prompt file from root, returning "" when the
// file does not exist or root is unset.
func ReadPromptFile(root, name string) (string, error) {
	if root == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
