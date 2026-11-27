package acp

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/wins/jaz/backend/internal/acpcredentials"
	"github.com/wins/jaz/backend/internal/codexauthbridge"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
)

func (m *Manager) sessionAgentConfig(agent, profileID string) (AgentConfig, error) {
	lookup := m.agents.AgentConfig
	if source, ok := m.agents.(agentMetadataSource); profileID != "" && ok {
		lookup = source.MetadataConfig
	}
	cfg, ok, err := lookup(agent)
	if err != nil {
		return AgentConfig{}, err
	}
	if !ok {
		return AgentConfig{}, fmt.Errorf("acp agent %q is not configured", agent)
	}
	if profileID == "" {
		return cfg, nil
	}
	if profileID != "acp:"+agent || (agent != AgentCodex && cloudKeyEnv(agent) == "") {
		return AgentConfig{}, errors.New("Cloud credential profile does not match the native agent")
	}
	if cfg.URL != "" || cfg.Local || cfg.BuiltinBridge {
		return AgentConfig{}, errors.New("Cloud credential profiles require a native agent subprocess")
	}
	if agent == AgentCodex {
		cfg.ModelProvider = provider.ProviderOpenAI
	}
	cfg.cloudCredentialProfileID = profileID
	return cfg, nil
}

func cloudKeyEnv(agent string) string {
	if agent == AgentAntigravity {
		return "GEMINI_API_KEY"
	}
	if agent == AgentClaude || agent == AgentGrok {
		spec, _ := AgentAPIKey(agent)
		return spec.TargetEnv
	}
	return ""
}

type cloudLaunchAuth struct {
	workspaceID string
	initial     acpcredentials.Profile
	resolve     func(context.Context, int64) (acpcredentials.Profile, error)
}

func (m *Manager) cloudLaunchAuth(ctx context.Context, agent string, cfg AgentConfig) (*cloudLaunchAuth, error) {
	profileID := cfg.cloudCredentialProfileID
	if profileID == "" {
		return nil, nil
	}
	if m.cfg.ResolveCloudProfile == nil {
		return nil, errors.New("connect this machine in Cloud workspace settings to use its agent profiles")
	}
	workspaceID := sessioncontext.WorkspaceID(ctx)
	resolve := func(ctx context.Context, rejectedVersion int64) (acpcredentials.Profile, error) {
		profile, err := m.cfg.ResolveCloudProfile(ctx, workspaceID, profileID, rejectedVersion)
		if err != nil {
			return acpcredentials.Profile{}, fmt.Errorf("resolve %s on this machine: %w", profileID, err)
		}
		if profile.Agent != agent || !profile.Valid() {
			return acpcredentials.Profile{}, errors.New("Cloud returned incompatible native credentials")
		}
		return profile, nil
	}
	initial, err := resolve(ctx, cfg.cloudRejectedVersion)
	if err != nil {
		return nil, err
	}
	if agent == AgentAntigravity && initial.OAuth != nil {
		return nil, errors.New("use the Antigravity Cloud profile with Cloud execution, or choose your local Antigravity sign-in")
	}
	return &cloudLaunchAuth{workspaceID: workspaceID, initial: initial, resolve: resolve}, nil
}

func (a *cloudLaunchAuth) prepare(ctx context.Context, agent, root string, env map[string]string, cfg *AgentConfig) (func(), error) {
	if a == nil {
		return func() {}, nil
	}
	profileRoot := filepath.Join(root, "acp", "cloud", fmt.Sprintf("%x", sha256.Sum256([]byte(a.workspaceID))))
	switch agent {
	case AgentCodex:
		codex := env["CODEX_PATH"]
		if codex == "" {
			var err error
			codex, err = resolveLoginExecutable(loginBinDirs(*cfg), "codex")
			if err != nil {
				return nil, errors.New("Codex CLI is required to use the workspace profile")
			}
		}
		home := filepath.Join(profileRoot, "codex")
		if err := os.MkdirAll(home, 0o700); err != nil {
			return nil, err
		}
		env["CODEX_HOME"] = home
		delete(env, "OPENAI_API_KEY")
		delete(env, "OPENAI_APIKEY")
		delete(env, "CODEX_API_KEY")
		delete(env, "CODEX_ACCESS_TOKEN")
		delete(env, "OPENAI_BASE_URL")
		return codexauthbridge.Install(ctx, env, codex, a.resolve)
	case AgentClaude:
		for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
			delete(env, key)
		}
		if a.initial.OAuth != nil {
			env["CLAUDE_CODE_OAUTH_TOKEN"] = a.initial.OAuth.AccessToken
			return func() {}, nil
		}
	case AgentAntigravity:
		delete(env, "GOOGLE_API_KEY")
		delete(env, "GOOGLE_GENAI_USE_VERTEXAI")
		delete(env, "GOOGLE_GEMINI_BASE_URL")
		geminiDir := filepath.Join(profileRoot, "gemini")
		settings := filepath.Join(geminiDir, "antigravity-cli")
		if err := os.MkdirAll(settings, 0o700); err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(settings, ".settings-*.json")
		if err != nil {
			return nil, err
		}
		defer os.Remove(file.Name())
		_, writeErr := file.WriteString(`{"modelProvider":"gemini"}`)
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return nil, err
		}
		if err := os.Rename(file.Name(), filepath.Join(settings, "settings.json")); err != nil {
			return nil, err
		}
		cfg.Args = append(slices.Clone(cfg.Args), "--gemini-dir", geminiDir)
	case AgentGrok:
		delete(env, "XAI_API_BASE_URL")
		delete(env, "CLI_CHAT_PROXY_BASE_URL")
		if !slices.Contains(cfg.Args, "--no-leader") {
			return nil, errors.New("the native Grok profile requires --no-leader")
		}
	}
	env[cloudKeyEnv(agent)] = a.initial.APIKey
	return func() {}, nil
}
