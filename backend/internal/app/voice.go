package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/voice/live"
)

func NewVoiceCredentials(store *sqlitestore.Store, catalog acp.AgentCatalog, providers provider.Source, layout runtimefiles.Layout, manager *acp.Manager) live.Credentials {
	var refreshMu sync.Mutex
	return func(connection string) (live.Credential, error) {
		if connection == live.ProviderAPI {
			cfg := providers.Providers()[provider.ProviderOpenAI]
			key := strings.TrimSpace(cfg.APIKey)
			keyEnv := provider.ConfiguredAPIKeyEnv(provider.ProviderOpenAI, cfg)
			if key == "" {
				key, _ = runtimeenv.Lookup(runtimeenv.Path(layout.Root), keyEnv)
			}
			if key == "" {
				key = strings.TrimSpace(os.Getenv(keyEnv))
			}
			if key == "" {
				return live.Credential{}, fmt.Errorf("add your OpenAI API key in Model Providers")
			}
			return live.Credential{Token: key}, nil
		}
		defaults, err := agentsettings.LoadEffectiveAgentDefaults(store, catalog)
		if err != nil {
			return live.Credential{}, err
		}
		cfg, _ := catalog.Agent(acp.AgentCodex)
		cfg.Auth = defaults.ACP[acp.AgentCodex].Auth
		cfg.ModelProvider = provider.ProviderOpenAI
		auth := acp.ProbeAgentAuth(acp.AgentCodex, cfg, layout.Root, nil)
		if !auth.Authenticated || auth.AuthKind != acp.AuthKindOAuth {
			return live.Credential{}, fmt.Errorf("sign in to OpenAI OAuth in Agents → Codex")
		}
		credential, err := readVoiceOAuth(auth.StoragePath)
		if err != nil {
			return live.Credential{}, err
		}
		credential.Refresh = func(ctx context.Context) (live.Credential, error) {
			refreshMu.Lock()
			defer refreshMu.Unlock()
			current, err := readVoiceOAuth(auth.StoragePath)
			if err == nil && current.Token != credential.Token {
				return current, nil
			}
			if err := manager.RefreshCodexOAuth(ctx, cfg, auth.AuthPath); err != nil {
				return live.Credential{}, err
			}
			return readVoiceOAuth(auth.StoragePath)
		}
		return credential, nil
	}
}

func readVoiceOAuth(path string) (live.Credential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return live.Credential{}, fmt.Errorf("voice needs a Codex file sign-in; sign in using Agents → Codex")
	}
	var stored struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if json.Unmarshal(data, &stored) != nil || stored.Tokens.AccessToken == "" || stored.Tokens.AccountID == "" {
		return live.Credential{}, fmt.Errorf("refresh your OpenAI OAuth sign-in in Agents → Codex")
	}
	return live.Credential{Token: stored.Tokens.AccessToken, AccountID: stored.Tokens.AccountID}, nil
}
