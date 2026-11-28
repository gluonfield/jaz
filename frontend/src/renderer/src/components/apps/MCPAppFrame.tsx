import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { CallToolResult } from '@modelcontextprotocol/client'
import type { AppBridge } from '@modelcontextprotocol/ext-apps/app-bridge'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { use, useEffect, useEffectEvent, useId, useRef, useState } from 'react'
import { z } from 'zod'
import { EmptyState } from '@/components/ui/EmptyState'
import { callMCPAppTool, deepLinkTarget, entrypointKey, mcpAppQuery, mcpEntrypointsQuery } from '@/lib/api/mcp'
import { useCloudSession } from '@/lib/cloudSession'
import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'
import { serveFile, type OpenedFile } from '@/lib/mcpAppFiles'
import { mcpAppHostContext } from '@/lib/mcpAppHost'
import { mcpAppKeyboard } from '@/lib/mcpAppKeyboard'
import { MCPAppThreadContext } from '@/lib/mcpAppThread'
import { useWindowEvent } from '@/lib/hooks/useWindowEvent'

const navigationParams = z.object({ path: z.string().regex(/^\/(?!\/)/), replace: z.boolean(), delta: z.number().int().optional() })
const TEXT_ONLY = { text: {} }

// The text of what a view sends; Jaz accepts no other content from views.
function viewText(content: { type: string; text?: string }[] = []): string {
  return content.flatMap((block) => (block.type === 'text' && block.text ? [block.text] : [])).join('\n\n').trim()
}

// Every sidebar app stays mounted over the content card, so opening its
// section is instant and finds the app as the user left it; only the active
// one is visible, and a deep link's path or a new call's input reaches it.
export function MCPApps() {
  const { activeWorkspaceId } = useCloudSession()
  const entrypoints = useQuery(mcpEntrypointsQuery).data ?? []
  const { pathname, search: { path, input, settings } } = useRouterState({ select: (state) => state.location })
  const navigate = useNavigate()
  return entrypoints
    .filter((entry) => entry.type === 'global')
    .map((entry) => {
      const key = entrypointKey(entry)
      const active = !settings && pathname === `/apps/${key}`
      return (
        <div key={`${activeWorkspaceId}:${key}`} className={`absolute inset-x-0 top-[var(--titlebar-height,0px)] bottom-0 bg-bg ${active ? '' : 'invisible'}`}>
          <MCPAppFrame
            app={entry}
            active={active}
            deepLink={active ? path : undefined}
            toolInput={active ? input : undefined}
            onNavigate={({ path: destination, replace, delta }) => {
              if (delta) {
                window.history.go(delta)
                return
              }
              if (destination === path && !input) return
              void navigate({
                to: '/apps/$serverId/$tool',
                params: { serverId: entry.server_id, tool: entry.tool },
                search: (previous) => ({ ...previous, path: destination, input: undefined }),
                replace: replace || !path || input !== undefined,
              })
            }}
            onDelivered={() => void navigate({
              to: '/apps/$serverId/$tool',
              params: { serverId: entry.server_id, tool: entry.tool },
              search: (previous) => ({ ...previous, input: undefined }),
              replace: true,
            })}
          />
        </div>
      )
    })
}

// Hosts one MCP App through the official AppBridge. An entrypoint opens by
// calling its tool with {} or, for a file viewer, the opened file; an agent's
// call shows with its own arguments and result, inline in the transcript. The
// app gets that input and result once it initializes. Tool calls reach the
// app's own server through Jaz, which holds the authenticated MCP session, so
// the sandboxed page never sees a token. Inside a thread the view can post a
// message to it, report its state for the user's next message and, from
// inline, open larger in the side panel.
export function MCPAppFrame({ app, active, inline = false, file, call, deepLink, toolInput, onNavigate, onDelivered }: {
  app: Pick<MCPEntrypoint, 'server_id' | 'tool'> & { title?: string }
  active: boolean
  inline?: boolean
  file?: OpenedFile
  call?: Pick<MCPAppEvent, 'arguments' | 'result'>
  deepLink?: string
  toolInput?: Record<string, unknown>
  onNavigate?: (location: z.infer<typeof navigationParams>) => void
  onDelivered?: () => void
}) {
  const { server_id: serverId, tool } = app
  const title = app.title ?? tool
  const query = useQuery(mcpAppQuery(serverId, tool))
  const frame = useRef<HTMLIFrameElement>(null)
  const bridge = useRef<AppBridge>(null)
  const appPath = useRef<string>(undefined)
  const html = query.data
  const thread = use(MCPAppThreadContext)
  const view = useId()
  const navigationEnabled = onNavigate !== undefined
  const displayMode = inline ? 'inline' : 'fullscreen'
  const expandable = inline && call !== undefined && thread?.expand !== undefined
  const reportsState = thread?.setModelContext !== undefined
  // The document paints before the app has the host theme, so it stays
  // transparent until the app reports ui/notifications/initialized.
  const [readyFor, setReadyFor] = useState<string>()
  // An inline app sizes its frame to its content.
  const [height, setHeight] = useState<number>()
  // An app that declares it can't render in this display mode stays hidden.
  const [unsupported, setUnsupported] = useState(false)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const delivered = useEffectEvent(() => onDelivered?.())
  const navigated = useEffectEvent((location: z.infer<typeof navigationParams>) => {
    appPath.current = location.path
    if (active && readyFor === html) onNavigate?.(location)
  })
  const hostContext = useEffectEvent((iframe: HTMLIFrameElement) => ({
    ...mcpAppHostContext(displayMode, expandable ? ['inline', 'fullscreen'] : [displayMode], iframe),
    ...(thread && { 'jaz/thread': { id: thread.id } }),
    ...(onNavigate && deepLink && { 'jaz/navigation': { path: deepLink } }),
  }))
  // A view's message goes to its thread as the user's. Outside a thread it
  // starts one on the new-thread page, with the agent, model and project
  // selected there. Only the view the user is working in may send.
  const sendMessage = useEffectEvent((text: string) => {
    if (!text || document.activeElement !== frame.current) return { isError: true }
    if (thread) thread.send(text)
    else void navigate({ to: '/new', state: { newThreadPrompt: { text, mode: 'send' } } })
    return {}
  })
  const reportState = useEffectEvent((text: string) => thread?.setModelContext?.(view, tool, text))
  const expand = useEffectEvent(() => {
    if (call) thread?.expand?.(view, app, call)
  })
  const callTool = useEffectEvent((input: Record<string, unknown>) =>
    callMCPAppTool(serverId, { name: tool, arguments: input }).catch(
      (error: Error): CallToolResult => ({ content: [{ type: 'text', text: error.message }], isError: true }),
    ),
  )

  useWindowEvent('message', (event) => {
    if (event.source !== frame.current?.contentWindow || document.activeElement !== frame.current) return
    if (event.data?.type !== 'jaz:navigation-shortcut' || typeof event.data.key !== 'string' || !/^(?:[1-9[\]]|BrowserBack|BrowserForward)$/.test(event.data.key)) return
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

  // A mounted app that is opened again reads a version older than a minute anew.
  useEffect(() => {
    if (active) void queryClient.prefetchQuery(mcpAppQuery(serverId, tool))
  }, [active, queryClient, serverId, tool])

  useEffect(() => {
    const iframe = frame.current
    const target = iframe?.contentWindow
    if (!iframe || !target || html === undefined) return
    let closed = false
    let observer: MutationObserver | undefined
    let resized: ResizeObserver | undefined
    const { input, result } = open()
    const connected = import('@modelcontextprotocol/ext-apps/app-bridge').then(async ({ AppBridge, PostMessageTransport }) => {
      if (closed) return undefined
      const created = new AppBridge(
        null,
        { name: 'Jaz', version: '1' },
        { openLinks: {}, serverTools: {}, logging: {}, message: TEXT_ONLY, ...(reportsState && { updateModelContext: TEXT_ONLY }), experimental: { ...(file && { 'openai/resource': {} }), ...(navigationEnabled && { 'jaz/navigation': {} }) } },
        { hostContext: hostContext(iframe) },
      )
      created.onmessage = async ({ content }) => sendMessage(viewText(content))
      created.onupdatemodelcontext = async ({ content }) => {
        reportState(viewText(content))
        return {}
      }
      // The expanded view is a second one in the side panel; this one stays inline.
      created.onrequestdisplaymode = async ({ mode }) => {
        if (mode === 'fullscreen' && expandable && document.activeElement === iframe) expand()
        return { mode: displayMode }
      }
      if (navigationEnabled) created.setNotificationHandler('jaz/notifications/navigation', { params: navigationParams }, navigated)
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
        if (!created.getAppCapabilities()?.experimental?.['jaz/navigation'] || !hostContext(iframe)['jaz/navigation']) {
          void created.sendToolInput({ arguments: input })
        }
        void result.then((value) => created.sendToolResult(value))
        // The app themes itself as it initializes; two frames let that paint land.
        requestAnimationFrame(() => requestAnimationFrame(() => setReadyFor(html)))
      }
      // Listen before the document loads so the app's first ui/initialize lands.
      await created.connect(new PostMessageTransport(target, target))
      iframe.srcdoc = inline ? html : html + mcpAppKeyboard
      const refresh = () => created.setHostContext(hostContext(iframe))
      observer = new MutationObserver(refresh)
      observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] })
      resized = new ResizeObserver(refresh)
      resized.observe(iframe)
      bridge.current = created
      return created
    })
    return () => {
      closed = true
      bridge.current = null
      observer?.disconnect()
      resized?.disconnect()
      void connected.then((created) => created?.close())
    }
  }, [html, serverId, tool, file, inline, displayMode, expandable, reportsState, navigationEnabled])

  // A navigation-aware app restores the path kept in Jaz's history. Other
  // apps receive the existing one-shot deep link.
  useEffect(() => {
    const current = bridge.current
    const iframe = frame.current
    if (!active || !current || !iframe || readyFor !== html) return
    if (current.getAppCapabilities()?.experimental?.['jaz/navigation']) {
      if (deepLink) {
        appPath.current = deepLink
        current.setHostContext(hostContext(iframe))
      } else if (appPath.current && !toolInput) {
        navigated({ path: appPath.current, replace: true })
      }
      return
    }
    if (!deepLink) return
    current.setHostContext({ ...hostContext(iframe), 'openai/deepLink': { url: deepLink } })
    current.setHostContext({ ...hostContext(iframe), 'openai/deepLink': undefined })
  }, [active, deepLink, toolInput, readyFor, html])

  // Navigation-aware apps replace the opening input with their resulting path.
  useEffect(() => {
    const current = bridge.current
    if (!toolInput || !current || readyFor !== html) return
    void current.sendToolInput({ arguments: toolInput })
    void callTool(toolInput).then((result) => current.sendToolResult(result))
    if (!current.getAppCapabilities()?.experimental?.['jaz/navigation']) delivered()
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
