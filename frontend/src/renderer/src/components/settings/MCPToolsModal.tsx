import { Modal } from '@/components/ui/Modal'
import type { MCPServer, MCPTool } from '@/lib/api/types'
import { mcpStatusText } from './MCPSettingsFormatting'

export function MCPToolsModal({
  server,
  onClose,
}: {
  server: MCPServer | null
  onClose: () => void
}) {
  return (
    <Modal
      open={server !== null}
      onClose={onClose}
      size="lg"
      title={server ? `${server.name} tools` : 'MCP tools'}
      description={server ? mcpStatusText(server) : undefined}
    >
      {server ? <MCPToolsList server={server} /> : null}
    </Modal>
  )
}

function MCPToolsList({ server }: { server: MCPServer }) {
  const tools = [...(server.tools ?? [])].sort((a, b) =>
    toolDisplayName(a).localeCompare(toolDisplayName(b)),
  )
  if (tools.length === 0) {
    return (
      <p className="rounded-card bg-surface px-3 py-3 text-[13px] text-ink-3">
        {server.status === 'connected'
          ? 'This server did not report any tools.'
          : 'Connect this server to load its tools.'}
      </p>
    )
  }
  return (
    <div className="space-y-2">
      {tools.map((tool) => (
        <MCPToolRow key={`${tool.name}:${tool.remote_name ?? ''}`} tool={tool} />
      ))}
    </div>
  )
}

function MCPToolRow({ tool }: { tool: MCPTool }) {
  const displayName = toolDisplayName(tool)
  return (
    <div className="min-w-0 rounded-control bg-surface px-3 py-2.5 select-text">
      <p className="truncate font-mono text-[12px] text-ink" title={displayName}>
        {displayName}
      </p>
      {tool.remote_name && tool.remote_name !== tool.name ? (
        <p className="mt-1 truncate text-[12px] text-ink-3" title={tool.name}>
          Jaz <span className="font-mono text-ink-2">{tool.name}</span>
        </p>
      ) : null}
      {tool.description ? (
        <p className="mt-2 text-[12px] leading-5 text-ink-2">{tool.description}</p>
      ) : null}
    </div>
  )
}

function toolDisplayName(tool: MCPTool): string {
  return tool.remote_name || tool.name
}
