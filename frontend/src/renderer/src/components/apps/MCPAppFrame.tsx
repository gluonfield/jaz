import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { CallToolResult } from '@modelcontextprotocol/client'
import type { AppBridge } from '@modelcontextprotocol/ext-apps/app-bridge'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { callMCPAppTool, deepLinkTarget, entrypointKey, mcpAppQuery, mcpEntrypointsQuery } from '@/lib/api/mcp'
import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'
import { serveFile, type OpenedFile } from '@/lib/mcpAppFiles'
import { mcpAppHostContext } from '@/lib/mcpAppHost'
import { mcpAppKeyboard } from '@/lib/mcpAppKeyboard'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'

// Every sidebar app stays mounted over the content card, so opening its
// section is instant and finds the app as the user left it; only the active
// one is visible, and a deep link's path or a new call's input reaches it.
export function MCPApps({ activeKey }: { activeKey?: string }) {
  const entrypoints = useQuery(mcpEntrypointsQuery).data ?? []
  const { path, input } = useSearch({ strict: false })
  const navigate = useNavigate()
  return entrypoints
    .filter((entry) => entry.type === 'global')
    .map((entry) => {
      const key = entrypointKey(entry)
      const active = key === activeKey
      return (
        <div key={key} className={`absolute inset-0 bg-bg ${active ? '' : 'invisible'}`}>
          <MCPAppFrame
            app={entry}
            active={active}
            deepLink={active ? path : undefined}
            toolInput={active ? input : undefined}
            onDelivered={() => void navigate({ to: '/apps/$serverId/$tool', params: { serverId: entry.server_id, tool: entry.tool }, replace: true })}
          />
        </div>
      )
    })
}

// Hosts one MCP App through the official AppBridge. An entrypoint opens by
// calling its tool with {} or, for a file viewer, the opened file; an agent's
// call shows inline with its own arguments and result. The app gets that
// input and result once it initializes. Tool calls reach the app's own server
// through Jaz, which holds the authenticated MCP session, so the sandboxed
// page never sees a token.
export function MCPAppFrame({ app, active, file, call, deepLink, toolInput, onDelivered }: {
  app: Pick<MCPEntrypoint, 'server_id' | 'tool'> & { title?: string }
  active: boolean
  file?: OpenedFile
  call?: Pick<MCPAppEvent, 'arguments' | 'result'>
  deepLink?: string
  toolInput?: Record<string, unknown>
  onDelivered?: () => void
}) {
  const { server_id: serverId, tool } = app
  const title = app.title ?? tool
  const query = useQuery(mcpAppQuery(serverId, tool))
  const frame = useRef<HTMLIFrameElement>(null)
  const bridge = useRef<AppBridge>(null)
  const html = query.data
  const inline = call !== undefined
  const displayMode = inline ? 'inline' : 'fullscreen'
  // The document paints before the app has the host theme, so it stays
  // transparent until the app reports ui/notifications/initialized.
  const [readyFor, setReadyFor] = useState<string>()
  // An inline app sizes its frame to its content.
  const [height, setHeight] = useState<number>()
  // An app that declares it can't render in this display mode stays hidden.
  const [unsupported, setUnsupported] = useState(false)
  const { refetch } = query
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const delivered = useEffectEvent(() => onDelivered?.())
  const callTool = useEffectEvent((input: Record<string, unknown>) =>
    callMCPAppTool(serverId, { name: tool, arguments: input }).catch(
      (error: Error): CallToolResult => ({ content: [{ type: 'text', text: error.message }], isError: true }),
    ),
  )

  useWindowEvent('message', (event) => {
    if (event.source !== frame.current?.contentWindow || document.activeElement !== frame.current) return
    if (event.data?.type !== 'jaz:navigation-shortcut' || typeof event.data.key !== 'string' || !/^[1-9]$/.test(event.data.key)) return
    window.dispatchEvent(new KeyboardEvent('keydown', { key: event.data.key, metaKey: true, cancelable: true }))
  }, active && !inline)

  // A deep link to a sidebar app of the app's own server opens that section
  // at the linked page; any other link opens in the browser.
  const followLink = useEffectEvent((url: string) => {
    const target = deepLinkTarget(url)
    const entry = target && queryClient.getQueryData(mcpEntrypointsQuery.queryKey)?.find(
      (point) => point.type === 'global' && point.server_id === serverId && point.tool === target.tool,
    )
    if (target && entry) {
      void navigate({ to: '/apps/$serverId/$tool', params: { serverId, tool: entry.tool }, search: { path: target.path } })
      return
    }
    window.open(url, '_blank', 'noopener,noreferrer')
  })

  // The opening input and result: the agent's call, or a call to the entrypoint.
  const open = useEffectEvent(() => {
    const input = call?.arguments ?? (file ? { file: { name: file.name, resourceUri: file.uri } } : {})
    const result = call ? Promise.resolve(call.result) : callTool(input)
    return { input, result }
  })

  useEffect(() => {
    if (active) void refetch()
  }, [active, refetch])

  useEffect(() => {
    const iframe = frame.current
    const target = iframe?.contentWindow
    if (!iframe || !target || html === undefined) return
    let closed = false
    let observer: MutationObserver | undefined
    const { input, result } = open()
    const connected = import('@modelcontextprotocol/ext-apps/app-bridge').then(async ({ AppBridge, PostMessageTransport }) => {
      if (closed) return undefined
      const created = new AppBridge(
        null,
        { name: 'Jaz', version: '1' },
        { openLinks: {}, serverTools: {}, logging: {}, ...(file && { experimental: { 'openai/resource': {} } }) },
        { hostContext: mcpAppHostContext(displayMode) },
      )
      // The host alone attests the opened file's path, which only the app's
      // server may see; an app's own openai/resource claim never passes.
      created.oncalltool = (params) =>
        callMCPAppTool(serverId, { ...params, _meta: { ...params._meta, 'openai/resource': file && { path: file.path } } })
      created.onopenlink = async ({ url }) => {
        followLink(url)
        return {}
      }
      created.onsizechange = ({ height }) => {
        if (inline && height !== undefined) setHeight(height)
      }
      if (file) serveFile(created, file)
      created.oninitialized = () => {
        const modes = created.getAppCapabilities()?.availableDisplayModes
        if (modes && !modes.includes(displayMode)) {
          setUnsupported(true)
          return
        }
        void created.sendToolInput({ arguments: input })
        void result.then((value) => created.sendToolResult(value))
        // The app themes itself as it initializes; two frames let that paint land.
        requestAnimationFrame(() => requestAnimationFrame(() => setReadyFor(html)))
      }
      // Listen before the document loads so the app's first ui/initialize lands.
      await created.connect(new PostMessageTransport(target, target))
      iframe.srcdoc = inline ? html : html + mcpAppKeyboard
      observer = new MutationObserver(() => created.setHostContext(mcpAppHostContext(displayMode)))
      observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] })
      bridge.current = created
      return created
    })
    return () => {
      closed = true
      bridge.current = null
      observer?.disconnect()
      void connected.then((created) => created?.close())
    }
  }, [html, serverId, tool, file, inline, displayMode])

  // A deep link reaches the running app as a host-context change, then clears,
  // so following the same link again changes the context again.
  useEffect(() => {
    const current = bridge.current
    if (!deepLink || !current || readyFor !== html) return
    current.setHostContext({ ...mcpAppHostContext(displayMode), 'openai/deepLink': { url: deepLink } })
    current.setHostContext({ ...mcpAppHostContext(displayMode), 'openai/deepLink': undefined })
    delivered()
  }, [deepLink, readyFor, html, displayMode])

  // A preview target reaches the running app as a new call of its tool, then
  // clears like a deep link.
  useEffect(() => {
    const current = bridge.current
    if (!toolInput || !current || readyFor !== html) return
    void current.sendToolInput({ arguments: toolInput })
    void callTool(toolInput).then((result) => current.sendToolResult(result))
    delivered()
  }, [toolInput, readyFor, html])

  if (unsupported) return null
  if (html === undefined && query.isError) {
    return inline ? null : (
      <EmptyState title={`Couldn't open ${title}`}>
        <p>{query.error.message}</p>
      </EmptyState>
    )
  }
  const ready = html !== undefined && readyFor === html
  return (
    <iframe
      ref={frame}
      title={title}
      sandbox="allow-scripts allow-forms allow-popups"
      allow="clipboard-write"
      style={inline ? { height: height ?? 0 } : undefined}
      className={`block border-0 transition-opacity duration-150 ${inline ? 'w-full' : 'size-full'} ${ready ? '' : 'pointer-events-none opacity-0'}`}
    />
  )
}
