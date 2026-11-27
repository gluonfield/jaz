package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

const mcpRefreshMethod = "_session/mcp/refresh"

const grokMCPUpdateMethod = "_x.ai/session/update_mcp_servers"

type MCPRevisionSource interface {
	Revision() uint64
	// ListedRevision reports no revision for a session whose MCP client never read the catalog.
	ListedRevision(session string) (uint64, bool)
}

func (m *Manager) SetMCPRevisionSource(source MCPRevisionSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpRevisionSource = source
}

func (m *Manager) mcpSource() MCPRevisionSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mcpRevisionSource
}

func (m *Manager) mcpRevision() uint64 {
	source := m.mcpSource()
	if source == nil {
		return 0
	}
	return source.Revision()
}

func supportedMCPRefresh(raw json.RawMessage, policy string) string {
	if !fullMCPPolicy(policy) {
		return ""
	}
	var response acpschema.InitializeResponse
	if json.Unmarshal(raw, &response) != nil || response.AgentCapabilities == nil {
		return ""
	}
	capabilities := response.AgentCapabilities
	if capabilities.MCPCapabilities == nil || !capabilities.MCPCapabilities.HTTP {
		return ""
	}
	refresh, _ := capabilities.Meta["mcpRefresh"].(map[string]any)
	if refresh["method"] == mcpRefreshMethod {
		return mcpRefreshMethod
	}
	if boolMeta(response.Meta, "grokShell") && boolMeta(response.Meta, "x.ai/mcp/sdk") {
		return grokMCPUpdateMethod
	}
	return ""
}

// An agent has the changed catalog only once its MCP client rereads it. Codex
// never does and other clients can miss tools/list_changed, so a relaunch that
// resumes the thread reads it instead. An agent that never read the catalog
// does not use it.
func (m *Manager) retireStaleCatalog(job *jobState, process *agentProcess) {
	source := m.mcpSource()
	if source == nil || process.mcpRefresh != "" || process.mcpRevision == source.Revision() {
		return
	}
	if listed, ok := source.ListedRevision(job.ID); !ok || listed == source.Revision() {
		return
	}
	if !m.subagentsRunning(job) && !m.hasLivePermissionWaiter(job.ID) {
		job.retire()
	}
}

func (m *Manager) refreshMCPBeforeTurn(ctx context.Context, job *jobState) error {
	m.mu.RLock()
	process := m.processes[job.ID]
	m.mu.RUnlock()
	if process == nil || process.mcpRefresh == "" {
		return nil
	}
	revision := m.mcpRevision()
	if process.mcpRevision == revision {
		return nil
	}
	params := map[string]any{"sessionId": job.ACPSession}
	if process.mcpRefresh == grokMCPUpdateMethod {
		servers, err := m.grokMCPServers(ctx, job, process.mcpPolicy, revision)
		if err != nil {
			return err
		}
		params["mcpServers"] = servers
	} else {
		params["serverNames"] = []string{"jaz_mcp"}
	}
	raw, err := process.peer.Call(ctx, process.mcpRefresh, params)
	if err != nil {
		return fmt.Errorf("refresh agent MCP tools: %w", err)
	}
	if process.mcpRefresh == grokMCPUpdateMethod {
		var response struct {
			Result struct {
				OK bool `json:"ok"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &response) != nil || !response.Result.OK {
			return fmt.Errorf("agent did not acknowledge MCP update")
		}
	}
	process.mcpRevision = revision
	return nil
}

func (m *Manager) grokMCPServers(ctx context.Context, job *jobState, policy string, revision uint64) ([]json.RawMessage, error) {
	servers, err := enabledHTTPMCPServers(mcpsession.With(ctx, job.ID), m.cfg.MCPStore, policy)
	if err != nil {
		return nil, err
	}
	for i, raw := range servers {
		var server httpMCPServer
		if err := json.Unmarshal(raw, &server); err != nil {
			return nil, err
		}
		if server.Name != "jaz_mcp" {
			continue
		}
		// Grok reconnects only changed configurations; version the Jaz-owned connection.
		server.Headers = append(server.Headers, mcpconfig.Header{Name: "X-Jaz-MCP-Revision", Value: strconv.FormatUint(revision, 10)})
		servers[i], err = json.Marshal(server)
		if err != nil {
			return nil, err
		}
	}
	return servers, nil
}
