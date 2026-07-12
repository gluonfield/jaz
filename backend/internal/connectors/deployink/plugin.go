package deployink

import "github.com/wins/jaz/backend/pkg/integrations"

const (
	ProviderID   = "deployink"
	ProviderName = "Deployink"
	RemoteMCPURL = "https://mcp.deployink.com"
)

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:          ProviderID,
		Name:        ProviderName,
		Description: "Deploy any app to the cloud. Run frontends, backends, and workers in any language, and provision databases.",
		Examples: []string{
			"Deploy this app to Ink",
			"Show my failing deployments and recent logs",
			"Create a Postgres database and wire it to my service",
		},
		Provider: integrations.Provider{
			ID:   ProviderID,
			Name: ProviderName,
		},
		Category: "deployment",
		Icon: integrations.PluginIcon{
			Kind:  integrations.PluginIconKindAsset,
			Value: "ink",
		},
		Auth: []integrations.AuthOption{{
			Kind:        integrations.AuthKindMCPConnection,
			Description: "Browser sign-in to Deployink for Jaz-managed deployment tools.",
		}},
		Capabilities: []integrations.Capability{
			integrations.CapabilityAct,
			integrations.CapabilityMCP,
		},
		Tools: tools(),
		RemoteMCP: &integrations.RemoteMCP{
			URL:          RemoteMCPURL,
			Status:       "available",
			OAuthSecrets: false,
		},
		ConnectionNotes: []string{
			"Connect Deployink once, then agents can deploy services, inspect logs, manage templates, and update DNS.",
		},
		Implementation: integrations.Implementation{
			Status: "available",
			Owner:  "jaz",
		},
	}
}

func tools() []integrations.PluginTool {
	return []integrations.PluginTool{
		tool("service_create", "Deploy a service from a git repository, image, or source upload.", integrations.ActionRiskWrite),
		tool("service_list", "List services in a workspace and project.", integrations.ActionRiskRead),
		tool("service_get", "Inspect service status, deployment details, logs, metrics, environment, domains, and volumes.", integrations.ActionRiskRead),
		tool("service_update", "Update service source, resources, ports, environment, replicas, or volume settings.", integrations.ActionRiskWrite),
		tool("service_delete", "Delete an Ink service.", integrations.ActionRiskDelete),
		tool("template_deploy", "Deploy a database or infrastructure template such as Postgres, Redis, MySQL, or MongoDB.", integrations.ActionRiskWrite),
		tool("template_instance_list", "List deployed template instances.", integrations.ActionRiskRead),
		tool("template_instance_get", "Read a template instance and its persisted outputs.", integrations.ActionRiskRead),
		tool("volume_list", "List persistent volumes in a workspace and project.", integrations.ActionRiskRead),
		tool("volume_resize", "Resize a persistent volume.", integrations.ActionRiskWrite),
		tool("volume_delete", "Delete a persistent volume.", integrations.ActionRiskDelete),
		tool("repo_create", "Create an Ink internal git repository.", integrations.ActionRiskWrite),
		tool("repo_get_token", "Create a push token for an Ink internal git repository.", integrations.ActionRiskWrite),
		tool("domain_add", "Attach a custom domain to a service.", integrations.ActionRiskWrite),
		tool("domain_remove", "Remove a custom domain from a service.", integrations.ActionRiskDelete),
		tool("dns_list_zones", "List delegated DNS zones.", integrations.ActionRiskRead),
		tool("dns_list_records", "List DNS records for a zone.", integrations.ActionRiskRead),
		tool("dns_add_record", "Add a DNS record to a delegated zone.", integrations.ActionRiskWrite),
		tool("dns_delete_record", "Delete a DNS record from a delegated zone.", integrations.ActionRiskDelete),
		tool("workspace_list", "List workspaces available to the signed-in user.", integrations.ActionRiskRead),
		tool("workspace_create", "Create a team workspace.", integrations.ActionRiskWrite),
		tool("project_list", "List projects in a workspace.", integrations.ActionRiskRead),
		tool("project_create", "Create a project in a workspace.", integrations.ActionRiskWrite),
		tool("chat_read", "Read workspace or project chat messages.", integrations.ActionRiskRead),
		tool("chat_send", "Send a workspace or project chat message.", integrations.ActionRiskWrite),
		tool("action_log_list", "Query workspace audit logs.", integrations.ActionRiskRead),
	}
}

func tool(name, description string, risk integrations.ActionRisk) integrations.PluginTool {
	return integrations.PluginTool{
		Name:        name,
		Description: description,
		Capability:  integrations.CapabilityAct,
		Risk:        risk,
	}
}
