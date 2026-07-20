package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/wins/jaz/backend/internal/processenv"
	modelprovider "github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/zalando/go-keyring"
)

const RefreshOwnerAgentCLI = "coding_agent_cli"

// The agy CLI keeps its Google OAuth token in the OS keyring under this
// entry, with ~/.gemini/antigravity-cli/antigravity-oauth-token as the
// fallback copy for hosts without a keyring.
const (
	antigravityKeyringService = "gemini"
	antigravityKeyringAccount = "antigravity"
)

const (
	AuthKindOAuth  = "oauth"
	AuthKindAPIKey = "api_key"
	AuthKindNone   = "none"
)

type AgentAPIKeySpec struct {
	SourceEnv string `json:"source_env"`
	TargetEnv string `json:"target_env"`
}

type resolvedAgentAuth struct {
	Config              AgentAuthConfig
	StoragePath         string
	Source              string
	Evidence            string
	Kind                string
	Authenticated       bool
	Reason              string
	APIKey              AgentAPIKeySpec
	APIKeySet           bool
	APIKeyValue         string
	APIKeyTargetAliases []string
}

func NormalizeAgentAuthConfig(name string, auth AgentAuthConfig) (AgentAuthConfig, error) {
	name = CanonicalAgentName(name)
	mode := strings.TrimSpace(auth.Mode)
	if mode == "" {
		mode = AuthModeAuto
	}
	switch mode {
	case AuthModeAuto, AuthModeExistingCLI, AuthModeJazProfile:
	default:
		return AgentAuthConfig{}, fmt.Errorf("acp agent %q auth mode %q is not supported", name, mode)
	}
	path := strings.TrimSpace(auth.Path)
	if name == AgentAntigravity && mode == AuthModeJazProfile {
		mode = AuthModeAuto
		path = ""
	}
	if mode == AuthModeAuto || mode == AuthModeJazProfile && hasJazAuthProfile(name) {
		path = ""
	}
	if name == AgentGrok {
		if mode == AuthModeJazProfile {
			return AgentAuthConfig{}, fmt.Errorf("acp agent %q auth mode %q is not supported because Grok has no explicit profile path", name, mode)
		}
		if path != "" {
			return AgentAuthConfig{}, fmt.Errorf("acp agent %q auth path is not supported because Grok has no explicit profile path", name)
		}
	}
	return AgentAuthConfig{
		Mode: mode,
		Path: path,
	}, nil
}

func DisconnectedAuthConfig(name string, current AgentAuthConfig) AgentAuthConfig {
	name = CanonicalAgentName(name)
	if hasJazAuthProfile(name) {
		return AgentAuthConfig{Mode: AuthModeJazProfile}
	}
	switch name {
	case AgentAntigravity:
		return AgentAuthConfig{Mode: AuthModeAuto}
	default:
		auth, err := NormalizeAgentAuthConfig(name, current)
		if err != nil {
			return AgentAuthConfig{Mode: AuthModeAuto}
		}
		return auth
	}
}

func LoginAuthConfig(name string, requested AgentAuthConfig) (AgentAuthConfig, error) {
	name = CanonicalAgentName(name)
	auth, err := NormalizeAgentAuthConfig(name, requested)
	if err != nil {
		return AgentAuthConfig{}, err
	}
	if hasJazAuthProfile(name) {
		if auth.Mode == AuthModeJazProfile {
			return auth, nil
		}
		return AgentAuthConfig{Mode: AuthModeJazProfile}, nil
	}
	if name == AgentAntigravity {
		return AgentAuthConfig{Mode: AuthModeExistingCLI}, nil
	}
	return auth, nil
}

func hasJazAuthProfile(name string) bool {
	switch CanonicalAgentName(name) {
	case AgentCodex, AgentClaude, AgentKimi, AgentQwen, AgentOpenCode:
		return true
	default:
		return false
	}
}

func resolveAgentAuth(name string, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	return resolveAgentAuthWithProviders(name, cfg, root, env, nil)
}

func resolveAgentAuthWithProviders(name string, cfg AgentConfig, root string, env map[string]string, providers map[string]modelprovider.ModelProviderConfig) resolvedAgentAuth {
	name = CanonicalAgentName(name)
	auth, err := NormalizeAgentAuthConfig(name, cfg.Auth)
	if err != nil {
		auth = AgentAuthConfig{Mode: AuthModeAuto}
	}
	switch name {
	case AgentCodex:
		return resolveCodexAuth(auth, cfg, root, env, providers)
	case AgentClaude:
		return resolveClaudeAuth(auth, cfg, root, env)
	case AgentKimi:
		return resolveKimiAuth(auth, cfg, root, env)
	case AgentQwen:
		return resolveQwenAuth(auth, cfg, root, env, providers)
	case AgentGrok:
		return resolveGrokAuth(auth, cfg, root, env)
	case AgentOpenCode:
		return resolveOpenCodeAuth(auth, cfg, root, env, providers)
	case AgentAntigravity:
		return resolveAntigravityAuth(auth, root, env)
	default:
		return resolvedAgentAuth{Config: auth}
	}
}

func resolveCodexAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string, providers map[string]modelprovider.ModelProviderConfig) resolvedAgentAuth {
	if meta, ok := codexProvider(cfg.ModelProvider, providers); ok {
		return resolveCodexProviderAuth(meta, root, env, providers)
	}
	layout := runtimefiles.New(root)
	existing := expandAuthPath(firstNonEmpty(existingAuthPath(auth), cfg.Env["CODEX_HOME"], env["CODEX_HOME"], os.Getenv("CODEX_HOME"), defaultHomePath(".codex")))
	jaz := layout.ACPCodexHome
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if codexAuthFileAvailable(jaz) {
			mode = AuthModeJazProfile
		} else if codexAuthFileAvailable(existing) || codexKeyringConfigured(existing) {
			mode = AuthModeExistingCLI
		}
	}
	path := jaz
	source := AuthModeJazProfile
	if mode == AuthModeExistingCLI {
		path = existing
		source = AuthModeExistingCLI
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode, Path: path},
		StoragePath: filepath.Join(path, "auth.json"),
		Source:      source,
	}
	apiKeyConfigured := status.resolveAPIKey(AgentCodex, root, env)
	switch {
	case codexAuthFileAvailable(path):
		status.markAuthenticated("auth_json", AuthKindOAuth)
	case codexKeyringConfigured(path):
		status.markAuthenticated("keyring_config", AuthKindOAuth)
	case apiKeyConfigured:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	default:
		status.Reason = "Codex login at " + filepath.Join(path, "auth.json") + " or " + status.APIKey.SourceEnv
	}
	return status
}

func resolveCodexProviderAuth(meta modelprovider.ModelProvider, root string, env map[string]string, providers map[string]modelprovider.ModelProviderConfig) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	keyEnv := strings.TrimSpace(meta.APIKeyEnv)
	status := resolvedAgentAuth{
		Config: AgentAuthConfig{Mode: AuthModeJazProfile, Path: layout.ACPCodexHome},
		Source: AuthModeJazProfile,
	}
	value := strings.TrimSpace(providers[codexProviderKeyID(meta.ID)].APIKey)
	if value == "" && keyEnv != "" {
		value = modelProviderKeyValue(root, env, keyEnv)
	}
	switch {
	case !meta.RequiresAPIKey:
		status.markAuthenticated("no_api_key_required", AuthKindNone)
	case keyEnv != "" && value != "":
		status.APIKey = AgentAPIKeySpec{SourceEnv: keyEnv, TargetEnv: keyEnv}
		status.APIKeyValue = value
		status.APIKeySet = true
		status.markAuthenticated(strings.ToLower(keyEnv)+"_env", AuthKindAPIKey)
	default:
		status.Reason = "Set " + firstNonEmpty(keyEnv, "the provider API key") + " in Settings > Model Providers to use " + firstNonEmpty(strings.TrimSpace(meta.Label), meta.ID) + " with Codex"
	}
	return status
}

func modelProviderKeyValue(root string, env map[string]string, keyEnv string) string {
	for _, name := range []string{keyEnv, apiKeyAlias(keyEnv)} {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if v := strings.TrimSpace(env[name]); v != "" {
			return v
		}
		if v, ok := runtimeenv.Lookup(runtimeenv.Path(root), name); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

func resolveClaudeAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	configuredExisting := firstNonEmpty(existingAuthPath(auth), cfg.Env["CLAUDE_CONFIG_DIR"], env["CLAUDE_CONFIG_DIR"], os.Getenv("CLAUDE_CONFIG_DIR"))
	existing := expandAuthPath(firstNonEmpty(configuredExisting, defaultHomePath(".claude")))
	jaz := layout.ACPClaudeConfig
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeJazProfile
		if claudeAuthFailureRecorded(jaz) || claudeAuthFileAvailable(jaz) {
			mode = AuthModeJazProfile
		} else if claudeAccountAuthAvailable(cfg.Env) || claudeAccountAuthAvailable(env) || claudeAuthFileAvailable(existing) {
			mode = AuthModeExistingCLI
		}
	}
	path := jaz
	source := AuthModeJazProfile
	if mode == AuthModeExistingCLI {
		path = ""
		if configuredExisting != "" {
			path = existing
		}
		source = AuthModeExistingCLI
	}
	storagePath := ""
	if path != "" {
		storagePath = claudeAuthPath(path)
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode, Path: path},
		StoragePath: storagePath,
		Source:      source,
	}
	accountAuthConfigured := source == AuthModeExistingCLI &&
		(claudeAccountAuthAvailable(cfg.Env) || claudeAccountAuthAvailable(env))
	apiKeyConfigured := status.resolveAPIKey(AgentClaude, root, env)
	rejectedLogin := source == AuthModeJazProfile && path != "" && claudeAuthFailureRecorded(path)
	switch {
	case accountAuthConfigured:
		status.markAuthenticated("env", AuthKindOAuth)
	case path != "" && !rejectedLogin && claudeAuthFileAvailable(path):
		status.markAuthenticated("claude_json", AuthKindOAuth)
	case apiKeyConfigured:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	case rejectedLogin:
		status.Reason = "Claude rejected the saved login; reconnect Claude"
	case source == AuthModeExistingCLI && configuredExisting == "":
		status.markAuthenticated("existing_cli", AuthKindOAuth)
	default:
		status.Reason = "Claude login at " + firstNonEmpty(storagePath, "the existing Claude CLI profile") + " or " + status.APIKey.SourceEnv
	}
	return status
}

func resolveGrokAuth(auth AgentAuthConfig, _ AgentConfig, root string, env map[string]string) resolvedAgentAuth {
	existing := defaultHomePath("")
	mode := auth.Mode
	if mode == AuthModeAuto || mode == "" {
		mode = AuthModeExistingCLI
	}
	storagePath := ""
	if existing != "" {
		storagePath = filepath.Join(existing, ".grok", "auth.json")
	}
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode},
		StoragePath: storagePath,
		Source:      AuthModeExistingCLI,
	}
	apiKeyConfigured := status.resolveAPIKey(AgentGrok, root, env)
	switch {
	case existing != "" && grokAuthFileAvailable(existing):
		status.markAuthenticated("auth_json", AuthKindOAuth)
	case apiKeyConfigured:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	default:
		status.Reason = "Grok login at " + grokAuthPath(existing) + " or " + status.APIKey.SourceEnv
	}
	return status
}

func resolveOpenCodeAuth(auth AgentAuthConfig, cfg AgentConfig, root string, env map[string]string, providers map[string]modelprovider.ModelProviderConfig) resolvedAgentAuth {
	layout := runtimefiles.New(root)
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: AuthModeJazProfile, Path: layout.ACPOpenCodeConfig},
		StoragePath: layout.ACPOpenCodeConfig,
		Source:      AuthModeJazProfile,
	}
	explicit := status.resolveAPIKey(AgentOpenCode, root, env)
	providerID := openCodeProviderID(cfg.ProviderQualifiedModel())
	meta, known := modelprovider.OpenCodeProviderByID(providerID)
	cfgProvider, configured := providers[providerID]
	keyEnv := strings.TrimSpace(meta.APIKeyEnv)
	if configured {
		if customKeyEnv := openCodeConfiguredProviderEnv(providerID, cfgProvider); customKeyEnv != "" {
			keyEnv = customKeyEnv
		}
	}
	requiresAPIKey := known && meta.RequiresAPIKey
	if !known && configured {
		requiresAPIKey = keyEnv != "" || strings.TrimSpace(cfgProvider.APIKey) != ""
	}
	switch {
	case known && !meta.RequiresAPIKey:
		status.markAuthenticated("no_api_key_required", AuthKindNone)
	case providerID == modelprovider.ProviderOpenRouter && explicit:
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
	case configured && strings.TrimSpace(cfgProvider.APIKey) != "":
		status.markAuthenticated("configured_provider_key", AuthKindAPIKey)
		status.APIKeySet = true
	case keyEnv != "" && providerAPIKeyConfigured(root, env, keyEnv, apiKeyAlias(keyEnv)):
		status.markAuthenticated(strings.ToLower(keyEnv)+"_env", AuthKindAPIKey)
		status.APIKeySet = true
	case configured && !requiresAPIKey:
		status.markAuthenticated("no_api_key_required", AuthKindNone)
	default:
		status.Reason = openCodeAPIKeyReason(providerID, meta, keyEnv, status.APIKey.SourceEnv, known || configured)
	}
	return status
}

func resolveAntigravityAuth(auth AgentAuthConfig, root string, env map[string]string) resolvedAgentAuth {
	mode := auth.Mode
	if mode == "" {
		mode = AuthModeAuto
	}
	cliPath := expandAuthPath(defaultHomePath(filepath.Join(".gemini", "antigravity-cli")))
	status := resolvedAgentAuth{
		Config:      AgentAuthConfig{Mode: mode},
		StoragePath: filepath.Join(cliPath, "antigravity-oauth-token"),
		Source:      mode,
	}
	cliAuthenticated := false
	if mode == AuthModeAuto || mode == AuthModeExistingCLI {
		cliAuthenticated = antigravityCLIAuthenticated(env)
	}
	if cliAuthenticated {
		status.Config = AgentAuthConfig{Mode: AuthModeExistingCLI, Path: cliPath}
		status.Source = AuthModeExistingCLI
		status.markAuthenticated("agy_models", AuthKindOAuth)
		return status
	}
	if mode == AuthModeExistingCLI {
		status.Config.Path = cliPath
	}
	status.Reason = "Antigravity CLI OAuth via agy"
	return status
}

// agy keeps its OAuth in the OS keyring, not a readable file, so the only
// reliable auth check is running `agy models`. A single settings load or save
// probes every agent — and antigravity twice per probe (env build + resolve) —
// so cache the result briefly to collapse those into one subprocess.
//
// Only the authenticated result is cached: a signed-in probe is the slow one
// (~1s), while a signed-out probe is cheap (~0.3s) and must stay fresh so the
// post-login verification sees the just-written credential instead of a stale no.
var antigravityAuthCache = struct {
	sync.Mutex
	authedUntil map[string]time.Time
}{authedUntil: map[string]time.Time{}}

const antigravityAuthTTL = 30 * time.Second

func antigravityCLIAuthenticated(env map[string]string) bool {
	path, err := executableInPath("agy", env["PATH"])
	if err != nil {
		path, err = ResolveExecutable("agy")
		if err != nil {
			return false
		}
	}
	key := path + "\x00" + env["HOME"]
	antigravityAuthCache.Lock()
	fresh := time.Now().Before(antigravityAuthCache.authedUntil[key])
	antigravityAuthCache.Unlock()
	if fresh {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "models")
	if len(env) > 0 {
		cmd.Env = processenv.List(env)
	}
	if cmd.Run() != nil {
		return false
	}
	antigravityAuthCache.Lock()
	antigravityAuthCache.authedUntil[key] = time.Now().Add(antigravityAuthTTL)
	antigravityAuthCache.Unlock()
	return true
}

// invalidateAntigravityAuthCache drops cached results so a disconnect is
// reflected on the next probe instead of lingering until the TTL expires.
func invalidateAntigravityAuthCache() {
	antigravityAuthCache.Lock()
	clear(antigravityAuthCache.authedUntil)
	antigravityAuthCache.Unlock()
}

func openCodeProviderID(model string) string {
	providerID := modelprovider.OpenCodeProviderIDFromModel(model)
	if providerID == "" {
		return modelprovider.ProviderOpenRouter
	}
	return providerID
}

func openCodeAPIKeyReason(providerID string, meta modelprovider.ModelProvider, keyEnv, explicitEnv string, configured bool) string {
	if !configured {
		return "OpenCode provider " + providerID + " is not configured"
	}
	label := firstNonEmpty(meta.Label, providerID)
	if providerID == modelprovider.ProviderOpenRouter {
		return "OpenCode " + label + " key via " + explicitEnv + " or " + keyEnv
	}
	return "OpenCode " + label + " key via " + keyEnv
}

func apiKeyAlias(key string) string {
	if strings.HasSuffix(key, "_API_KEY") {
		return strings.TrimSuffix(key, "_API_KEY") + "_APIKEY"
	}
	return ""
}

// RemoveOwnedCredential deletes an agent's OAuth credential, but ONLY when Jaz
// owns it — stored in Jaz's own profile (under root) — or when the CLI login is
// the agent's sole auth (Grok's ~/.grok/auth.json, Antigravity's agy token).
// It never deletes the user's global ~/.claude.json / ~/.codex config. A no-op
// when the path is empty, absent, or not Jaz-owned.
func RemoveOwnedCredential(name, storagePath, root string) error {
	name = CanonicalAgentName(name)
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return nil
	}
	if !pathUnderRoot(storagePath, root) && name != AgentGrok && name != AgentAntigravity {
		return nil
	}
	// Credentials are always files; a directory StoragePath (e.g. OpenCode's
	// config dir) holds no removable login.
	if info, err := os.Stat(storagePath); err == nil && info.IsDir() {
		return nil
	}
	if name == AgentClaude {
		return removeClaudeProfileCredentials(filepath.Dir(storagePath))
	}
	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	// agy stores its OAuth token in the OS keyring; the file is a fallback copy.
	// Best-effort: headless hosts have no keyring and agy uses the file there.
	if name == AgentAntigravity {
		_ = keyring.Delete(antigravityKeyringService, antigravityKeyringAccount)
		invalidateAntigravityAuthCache()
	}
	return nil
}

// pathUnderRoot resolves symlinks before comparing so a symlinked directory
// inside root cannot smuggle a global credential path past the ownership check.
func pathUnderRoot(path, root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(resolved, filepath.Base(path))
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func grokAuthPath(home string) string {
	if strings.TrimSpace(home) == "" {
		return "~/.grok/auth.json"
	}
	return filepath.Join(home, ".grok", "auth.json")
}

func existingAuthPath(auth AgentAuthConfig) string {
	if auth.Mode == AuthModeExistingCLI {
		return auth.Path
	}
	return ""
}

func (a *resolvedAgentAuth) resolveAPIKey(name, root string, env map[string]string) bool {
	a.APIKey, _ = resolveAgentAPIKeySpec(name)
	value, ok := explicitAgentAPIKey(name, root, env)
	a.APIKeySet = ok
	a.APIKeyValue = value
	return ok
}

func (a *resolvedAgentAuth) markAuthenticated(evidence, kind string) {
	a.Authenticated = true
	a.Evidence = evidence
	a.Kind = kind
}

func (a resolvedAgentAuth) APIKeyBinding() (string, string, bool) {
	if a.Kind != AuthKindAPIKey || strings.TrimSpace(a.APIKeyValue) == "" || strings.TrimSpace(a.APIKey.TargetEnv) == "" {
		return "", "", false
	}
	return a.APIKey.TargetEnv, a.APIKeyValue, true
}

func (a resolvedAgentAuth) BindAPIKeyEnv(env map[string]string) {
	target, value, ok := a.APIKeyBinding()
	if !ok {
		return
	}
	env[target] = value
	for _, extra := range a.APIKeyTargetAliases {
		if target := strings.TrimSpace(extra); target != "" {
			env[target] = value
		}
	}
}

func AgentAPIKey(name string) (AgentAPIKeySpec, bool) {
	return resolveAgentAPIKeySpec(name)
}

func resolveAgentAPIKeySpec(name string) (AgentAPIKeySpec, bool) {
	switch CanonicalAgentName(name) {
	case AgentCodex:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_CODEX_API_KEY", TargetEnv: "OPENAI_API_KEY"}, true
	case AgentClaude:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_CLAUDE_API_KEY", TargetEnv: "ANTHROPIC_API_KEY"}, true
	case AgentGrok:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_GROK_API_KEY", TargetEnv: "XAI_API_KEY"}, true
	case AgentQwen:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_QWEN_API_KEY", TargetEnv: "BAILIAN_CODING_PLAN_API_KEY"}, true
	case AgentOpenCode:
		return AgentAPIKeySpec{SourceEnv: "JAZ_ACP_OPENCODE_API_KEY", TargetEnv: "OPENROUTER_API_KEY"}, true
	default:
		return AgentAPIKeySpec{}, false
	}
}

func explicitAgentAPIKey(name, root string, env map[string]string) (string, bool) {
	spec, ok := resolveAgentAPIKeySpec(name)
	if !ok {
		return "", false
	}
	if value := strings.TrimSpace(env[spec.SourceEnv]); value != "" {
		return value, true
	}
	if value, ok := runtimeenv.Lookup(runtimeenv.Path(root), spec.SourceEnv); ok {
		return value, true
	}
	if value := strings.TrimSpace(os.Getenv(spec.SourceEnv)); value != "" {
		return value, true
	}
	return "", false
}

func providerAPIKeyConfigured(root string, env map[string]string, canonical string, aliases ...string) bool {
	if strings.TrimSpace(canonical) == "" {
		return false
	}
	if strings.TrimSpace(env[canonical]) != "" {
		return true
	}
	if value, ok := runtimeenv.Lookup(runtimeenv.Path(root), canonical); ok && strings.TrimSpace(value) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv(canonical)) != "" {
		return true
	}
	for _, alias := range aliases {
		if strings.TrimSpace(env[alias]) != "" || strings.TrimSpace(os.Getenv(alias)) != "" {
			return true
		}
	}
	return false
}

func codexAuthFileAvailable(home string) bool {
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return false
	}
	var auth struct {
		Mode   string `json:"auth_mode"`
		Tokens struct {
			Access  string `json:"access_token"`
			Refresh string `json:"refresh_token"`
		} `json:"tokens"`
	}
	return json.Unmarshal(data, &auth) == nil && auth.Mode == "chatgpt" && auth.Tokens.Access != "" && auth.Tokens.Refresh != ""
}

func codexKeyringConfigured(home string) bool {
	var config struct {
		CredentialsStore string `toml:"cli_auth_credentials_store"`
	}
	data, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		return false
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return false
	}
	store := strings.ToLower(strings.TrimSpace(config.CredentialsStore))
	return store == "keyring" || store == "auto"
}

func claudeAuthFileAvailable(configDir string) bool {
	return claudeOAuthAccountAvailable(claudeAuthPath(configDir)) ||
		fileExists(filepath.Join(configDir, ".credentials.json"))
}

func claudeAuthPath(configDir string) string {
	return filepath.Join(configDir, ".claude.json")
}

func claudeOAuthAccountAvailable(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var value claudeConfigJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return false
	}
	return strings.TrimSpace(value.OAuthAccount.AccountUUID) != ""
}

type claudeConfigJSON struct {
	OAuthAccount claudeOAuthAccountJSON `json:"oauthAccount"`
}

type claudeOAuthAccountJSON struct {
	AccountUUID string `json:"accountUuid"`
}

func claudeAccountAuthAvailable(env map[string]string) bool {
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if strings.TrimSpace(env[key]) != "" {
			return true
		}
	}
	return false
}

func grokAuthFileAvailable(home string) bool {
	return fileExists(filepath.Join(home, ".grok", "auth.json"))
}

func defaultHomePath(child string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if strings.TrimSpace(child) == "" {
		return home
	}
	return filepath.Join(home, child)
}

func expandAuthPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "~" {
		return defaultHomePath("")
	}
	if strings.HasPrefix(path, "~/") {
		if home := defaultHomePath(""); home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
