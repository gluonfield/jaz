package server

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	acpSlowProbeRefreshAfter = 30 * time.Second
	acpSlowProbeUnauthTTL    = 15 * time.Second
)

type acpAuthProbeCache struct {
	mu      sync.Mutex
	entries map[string]*acpAuthProbeEntry
}

type acpAuthProbeEntry struct {
	fingerprint string
	generation  uint64
	hasResult   bool
	auth        acp.AgentAuthStatus
	readiness   acp.Readiness
	probedAt    time.Time
	inFlight    bool
}

type acpAgentProbeSnapshot struct {
	name      string
	cfg       acp.AgentConfig
	command   string
	err       error
	auth      acp.AgentAuthStatus
	readiness acp.Readiness
	checking  bool
}

func (c *acpAuthProbeCache) entryLocked(agent string) *acpAuthProbeEntry {
	if c.entries == nil {
		c.entries = map[string]*acpAuthProbeEntry{}
	}
	entry := c.entries[agent]
	if entry == nil {
		entry = &acpAuthProbeEntry{}
		c.entries[agent] = entry
	}
	return entry
}

func acpAuthProbeFingerprint(root string, cfg acp.AgentConfig) string {
	return strings.Join([]string{
		root,
		cfg.Auth.Mode,
		cfg.Auth.Path,
		cfg.Command,
		cfg.AdapterBinDir,
		cfg.LoginBinDir,
		cfg.ManagedAdapter,
		cfg.ManagedTool,
		cfg.ModelProvider,
		os.Getenv("HOME"),
		os.Getenv("PATH"),
	}, "\x00")
}

// WarmSlowACPAuth starts non-blocking background probes for slow CLI agents so
// their auth status and (for Antigravity) native model list are warm before the
// composer asks for them.
func (s *Server) WarmSlowACPAuth() {
	var defaults agentsettings.AgentDefaults
	if store, ok := s.Store.(storage.SettingsStorage); ok {
		loaded, err := s.loadAgentSettings(store)
		if err == nil {
			defaults = loaded
		}
	}
	if defaults.ACP == nil {
		defaults = s.agentSettingsSeed()
	}
	providers := s.modelProviders()
	root := s.runtimeRoot()
	for _, name := range s.allACPAgentNames() {
		cfg, _, err := s.acpProbeConfig(name, defaults)
		if err != nil || !acp.IsSlowAuthProbe(name, cfg) {
			continue
		}
		_, _, _ = s.probeACPAuthNonBlocking(name, cfg, root, providers)
	}
}

func (s *Server) invalidateACPAuthProbe(agent string) {
	agent = acp.CanonicalAgentName(agent)
	s.acpAuthProbes.mu.Lock()
	entry := s.acpAuthProbes.entryLocked(agent)
	entry.generation++
	entry.hasResult = false
	entry.inFlight = false
	s.acpAuthProbes.mu.Unlock()
}

func (s *Server) invalidateAllSlowACPAuthProbes() {
	s.acpAuthProbes.mu.Lock()
	for _, entry := range s.acpAuthProbes.entries {
		entry.generation++
		entry.hasResult = false
		entry.inFlight = false
	}
	s.acpAuthProbes.mu.Unlock()
}

func (s *Server) recordACPAuthProbe(agent string, cfg acp.AgentConfig, auth acp.AgentAuthStatus, readiness acp.Readiness) {
	agent = acp.CanonicalAgentName(agent)
	if !acp.IsSlowAuthProbe(agent, cfg) {
		return
	}
	fp := acpAuthProbeFingerprint(s.runtimeRoot(), cfg)
	s.acpAuthProbes.mu.Lock()
	entry := s.acpAuthProbes.entryLocked(agent)
	entry.generation++
	entry.fingerprint = fp
	entry.hasResult = true
	entry.auth = auth
	entry.readiness = readiness
	entry.probedAt = time.Now()
	entry.inFlight = false
	s.acpAuthProbes.mu.Unlock()
}

func (s *Server) probeAllACPAuth(defaults agentsettings.AgentDefaults) []acpAgentProbeSnapshot {
	names := s.allACPAgentNames()
	providers := s.modelProviders()
	root := s.runtimeRoot()
	results := make([]acpAgentProbeSnapshot, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(idx int, agentName string) {
			defer wg.Done()
			agentName = acp.CanonicalAgentName(agentName)
			cfg, command, err := s.acpProbeConfig(agentName, defaults)
			if err != nil {
				results[idx] = acpAgentProbeSnapshot{name: agentName, command: command, err: err}
				return
			}
			auth, readiness, checking := s.probeACPAuthNonBlocking(agentName, cfg, root, providers)
			results[idx] = acpAgentProbeSnapshot{
				name:      agentName,
				cfg:       cfg,
				command:   command,
				auth:      auth,
				readiness: readiness,
				checking:  checking,
			}
		}(i, name)
	}
	wg.Wait()
	return results
}

func (s *Server) probeACPAuthNonBlocking(
	name string,
	cfg acp.AgentConfig,
	root string,
	providers map[string]provider.ModelProviderConfig,
) (acp.AgentAuthStatus, acp.Readiness, bool) {
	name = acp.CanonicalAgentName(name)
	if !acp.IsSlowAuthProbe(name, cfg) {
		auth, readiness := acp.ProbeAuthAndReadinessWithProviders(name, cfg, root, nil, providers)
		return auth, readiness, false
	}
	pending := acp.PendingAgentAuthStatus(name, cfg, root, nil)
	if !slowProbeRequiresSubprocess(name, pending) {
		auth, readiness := acp.ProbeAuthAndReadinessWithProviders(name, cfg, root, nil, providers)
		return auth, readiness, false
	}

	fp := acpAuthProbeFingerprint(root, cfg)
	now := time.Now()

	s.acpAuthProbes.mu.Lock()
	entry := s.acpAuthProbes.entryLocked(name)
	if entry.fingerprint != fp {
		entry.generation++
		entry.fingerprint = fp
		entry.hasResult = false
		entry.inFlight = false
	}
	needsProbe := !entry.hasResult
	if entry.hasResult {
		refreshAfter := acpSlowProbeUnauthTTL
		if entry.auth.Authenticated {
			refreshAfter = acpSlowProbeRefreshAfter
		}
		needsProbe = now.Sub(entry.probedAt) >= refreshAfter
	}
	if needsProbe && !entry.inFlight {
		entry.inFlight = true
		gen := entry.generation
		probeEnv := map[string]string{
			"HOME": os.Getenv("HOME"),
			"PATH": os.Getenv("PATH"),
		}
		go s.runSlowACPAuthProbe(name, cfg, root, probeEnv, providers, fp, gen)
	}
	if entry.hasResult {
		auth, readiness := entry.auth, entry.readiness
		s.acpAuthProbes.mu.Unlock()
		return auth, readiness, false
	}
	s.acpAuthProbes.mu.Unlock()
	return pending, acp.Readiness{}, true
}

func slowProbeRequiresSubprocess(name string, pending acp.AgentAuthStatus) bool {
	if !pending.LoginCommandAvailable {
		return false
	}
	if name == acp.AgentMuse && (pending.APIKeyConfigured || strings.TrimSpace(os.Getenv("META_API_KEY")) != "") {
		return false
	}
	return true
}

func (s *Server) runSlowACPAuthProbe(
	name string,
	cfg acp.AgentConfig,
	root string,
	env map[string]string,
	providers map[string]provider.ModelProviderConfig,
	fp string,
	gen uint64,
) {
	auth, readiness := acp.ProbeAuthAndReadinessWithProviders(name, cfg, root, env, providers)
	s.acpAuthProbes.mu.Lock()
	defer s.acpAuthProbes.mu.Unlock()
	entry := s.acpAuthProbes.entryLocked(name)
	if entry.generation != gen || entry.fingerprint != fp {
		return
	}
	entry.auth = auth
	entry.readiness = readiness
	entry.hasResult = true
	entry.probedAt = time.Now()
	entry.inFlight = false
}
