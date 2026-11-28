import { MarkdownManager } from '@tiptap/markdown'
import { findFileReferences, isHTMLPath, isMarkdownPath, isSpreadsheetPath, isWordPath, type FileReference } from '@shared/fileReader'
import { itemLabel } from '@/components/canvas/board'
import { isAppToolCall, parseAppToolOutput } from '@/components/session/appTool'
import type { LiveExchange } from '@/components/session/liveTranscript'
import { isArtifactToolName } from '@/components/session/toolVisibility'
import type { CanvasDocument } from '@/lib/api/canvas'
import type { ACPToolCall, ChatMessage, Session, SessionEvent } from '@/lib/api/types'
import { artifactErrorForDisplay, artifactInputFromEvent, artifactToolError, parseArtifactToolArgs } from '@/lib/artifacts'
import { standaloneFileReference } from '@/lib/markdownDocuments'
import { markdownLinkTarget } from '@/lib/markdownLinks'
import { messageText } from '@/lib/messageText'
import { artifactTab, publishedAppTab, type PublishedAppRef, type ThreadContentTab } from '@/lib/sidePanelTabs'

type ThreadPanelItem = { id: string; label: string; tab: ThreadContentTab | { kind: 'file'; file: FileReference } }

const markdown = new MarkdownManager({ extensions: [] }).instance

export function threadPanelItems({ session, messages, events, live, apps, canvas }: {
  session: Pick<Session, 'id' | 'runtime_ref'>
  messages: ChatMessage[]
  events: SessionEvent[]
  live?: LiveExchange | null
  apps?: PublishedAppRef[]
  canvas?: CanvasDocument
}): ThreadPanelItem[] {
  const items = new Map<string, ThreadPanelItem>()
  const add = (tab: ThreadContentTab, label: string, id = tab.id) => items.set(id, { id, label, tab })
  const files = { sessionId: session.id, documentPath: session.runtime_ref?.cwd ? `${session.runtime_ref.cwd}/` : undefined }
  const canvasAvailable = session.runtime_ref?.artifact_surface !== 'canvas'
  const addDocument = (file: FileReference, name?: string) => {
    const { path } = file
    if (!isMarkdownPath(path) && !isWordPath(path) && !isSpreadsheetPath(path) && !isHTMLPath(path) && !/\.(pdf|svg|txt)$/i.test(path)) return
    const id = `file:${path}`
    const label = name || path.split(/[\\/]/).pop() || path
    items.set(id, { id, label, tab: { kind: 'file', file } })
  }
  const addCanvasTarget = (page: string, item: string | undefined, label: string) => {
    add({ id: 'canvas', kind: 'canvas', target: { page, item } }, label, `canvas:${JSON.stringify([page, item])}`)
  }
  const addMarkdown = (text?: string) => {
    if (!text || !/[.\/\\?]/.test(text)) return
    const linkLabels = new WeakSet<object>()
    markdown.walkTokens(markdown.lexer(text), (token) => {
      if (linkLabels.has(token)) return
      if (token.type === 'link') {
        if (token.tokens) markdown.walkTokens(token.tokens, (child) => { linkLabels.add(child) })
        const { href, text: label } = token
        if (/^[@$]/.test(label)) return
        const appId = /^\/apps\/([a-z0-9]{26})(?:[?#].*)?$/.exec(href)?.[1]
        if (appId) {
          add(publishedAppTab({ id: appId, title: label }), label)
          return
        }
        const target = markdownLinkTarget(href, label, files)
        if (target?.kind === 'file') addDocument(target.file, label)
        if (canvasAvailable && target?.kind === 'canvas' && target.sessionId === session.id && target.page) {
          const page = canvas?.pages.find((entry) => entry.id === target.page)
          if (page && (!target.item || page.nodes.some((node) => node.id === target.item))) {
            addCanvasTarget(target.page, target.item, label)
          }
        }
      } else if (token.type === 'paragraph' && token.tokens?.length === 1 && token.tokens[0].type === 'text') {
        const file = standaloneFileReference(token.text, files.documentPath)
        if (file) addDocument(file)
      } else if (token.type === 'text' && !token.tokens) {
        for (const { reference } of findFileReferences(token.text)) addDocument(reference)
      }
    })
  }
  const addTool = (call: Pick<ACPToolCall, 'tool_name' | 'title' | 'status' | 'raw_input' | 'raw_output'>) => {
    if (isAppToolCall(call)) {
      const result = parseAppToolOutput(call.raw_output)
      if (result && 'app' in result) add(publishedAppTab(result.app), result.app.title)
    } else if (call.status === 'completed' && (isArtifactToolName(call.tool_name) || isArtifactToolName(call.title)) && !artifactErrorForDisplay(artifactToolError(call.raw_output))) {
      const input = parseArtifactToolArgs(call.raw_input)
      if (input) add(artifactTab(input), input.title)
    }
  }

  if (canvasAvailable) {
    for (const page of canvas?.pages ?? []) {
      if (!page.nodes.length && canvas?.pages.length === 1) continue
      addCanvasTarget(page.id, undefined, page.name)
      for (const item of page.nodes) addCanvasTarget(page.id, item.id, itemLabel(item))
    }
  }
  for (const message of messages) {
    if (message.role !== 'assistant' && message.role !== 'user') continue
    addMarkdown(messageText(message))
    for (const block of message.blocks ?? []) {
      if (block.type === 'tool' && message.role === 'assistant') {
        addTool({ tool_name: block.name, raw_input: block.input_json, raw_output: block.result, status: 'completed' })
      }
    }
  }
  for (const event of events) {
    if (event.session_id !== session.id || event.acp && event.acp.id !== session.id) continue
    const presentation = event.artifact
    if (presentation?.kind === 'artifact') {
      const input = artifactInputFromEvent(presentation.artifact)
      if (input) add(artifactTab(input), input.title)
    } else if (presentation?.kind === 'canvas' && canvasAvailable && canvas?.pages.length) {
      add({ id: 'canvas', kind: 'canvas' }, 'Canvas')
    }
    addMarkdown(event.type === 'acp_message' ? event.content : event.acp?.assistant)
    for (const call of event.acp?.tool_calls ?? []) addTool(call)
  }
  if (live) {
    addMarkdown(live.user)
    addMarkdown(live.assistant)
    for (const tool of live.tools) {
      addTool({ tool_name: tool.name, raw_input: tool.args, raw_output: tool.result, status: tool.result === undefined ? 'running' : 'completed' })
    }
  }
  for (const app of apps ?? []) add(publishedAppTab(app), app.title)
  return [...items.values()]
}
