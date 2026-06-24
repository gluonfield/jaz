import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
import { uploadSessionAttachment } from '@/lib/api/sessions'
import { streamSessionMessage, type AgentStreamEvent } from '@/lib/api/stream'
import type { ChatMessage, MessageBlock } from '@/lib/api/types'
import { contextInputs } from '@/lib/messageContext'
import { keys } from '@/lib/query/keys'
import { preparedSendMessage, type ComposerContext, type SendMessageOptions } from '@/lib/sendMessage'
import { isHiddenToolName } from './toolVisibility'

export interface LiveExchange {
  user: string
  at: string
  planRequested: boolean
  contexts: ComposerContext[]
  attachments: LiveAttachment[]
  reasoning: string
  assistant: string
  tools: LiveTool[]
  error?: string
}

export interface LiveAttachment {
  id?: string
  name: string
  uri?: string
  mime_type?: string
  size?: number
  server_path?: string
  uploading?: boolean
}

export interface LiveTool {
  key: string
  name: string
  args?: string
  result?: string
}

export function useLiveSessionSend({
  sessionId,
  streamingRef,
  onCriticalError,
}: {
  sessionId: string
  streamingRef: MutableRefObject<boolean>
  onCriticalError: (message: string) => void
}) {
  const queryClient = useQueryClient()
  const [live, setLive] = useState<LiveExchange | null>(null)
  const [streaming, setStreaming] = useState(false)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => () => abortRef.current?.abort(), [sessionId])

  const send = useCallback((text: string, options: SendMessageOptions = {}) => {
    const controller = new AbortController()
    const files = options.files ?? []
    const draftAttachments = options.attachments ?? []
    const draftContexts = options.contexts ?? []
    const contextAttachments = liveContextAttachments(draftContexts)
    abortRef.current = controller
    setLive({
      user: text,
      at: new Date().toISOString(),
      planRequested: Boolean(options.planRequested),
      contexts: draftContexts,
      attachments: [
        ...draftAttachments,
        ...contextAttachments,
        ...files.map((file) => ({ name: file.name, size: file.size, uploading: true })),
      ],
      reasoning: '',
      assistant: '',
      tools: [],
    })
    setStreaming(true)
    streamingRef.current = true

    ;(async () => {
      const attachments = files.length
        ? await Promise.all(files.map((file) => uploadSessionAttachment(sessionId, file, controller.signal)))
        : []
      if (attachments.length) {
        setLive((prev) =>
          prev ? { ...prev, attachments: [...draftAttachments, ...contextAttachments, ...attachments] } : prev,
        )
      }
      const prepared = preparedSendMessage(options, attachments)
      await streamSessionMessage({
        sessionId,
        message: text,
        contexts: prepared.contexts,
        attachmentIds: prepared.attachmentIds,
        planRequested: options.planRequested,
        signal: controller.signal,
        onEvent: (event) => {
          if (event.type === 'error') {
            onCriticalError(event.error || 'Something went wrong.')
          }
          setLive((prev) => (prev ? mergeLiveStreamEvent(prev, event) : prev))
        },
      })
    })()
      .catch((err: Error) => {
        if (controller.signal.aborted) return
        onCriticalError(err.message || 'Something went wrong.')
        setLive((prev) =>
          prev ? { ...prev, attachments: finishLiveAttachments(prev.attachments), error: err.message } : prev,
        )
      })
      .finally(async () => {
        setStreaming(false)
        streamingRef.current = false
        abortRef.current = null
        await queryClient.refetchQueries({ queryKey: keys.sessionMessages(sessionId) })
        queryClient.invalidateQueries({ queryKey: keys.sidebarSessions })
        queryClient.invalidateQueries({ queryKey: keys.allSessions })
        queryClient.invalidateQueries({ queryKey: keys.usage })
        queryClient.invalidateQueries({ queryKey: keys.sessionRepo(sessionId) })
        setLive((prev) => (prev?.error ? { ...prev, error: undefined } : null))
      })
  }, [onCriticalError, queryClient, sessionId, streamingRef])

  const abort = useCallback(() => {
    abortRef.current?.abort()
  }, [])

  return { live, streaming, send, abort }
}

export function liveExchangeSize(live: LiveExchange | null): number {
  return live
    ? live.user.length +
        live.contexts.length +
        live.reasoning.length +
        live.assistant.length +
        live.tools.length +
        live.attachments.length +
        (live.error?.length ?? 0)
    : 0
}

export function liveUserMessage(live: LiveExchange, seq: number): ChatMessage {
  return {
    seq,
    role: 'user',
    content: live.user,
    blocks: [
      ...contextInputs(live.contexts).map<MessageBlock>((context) =>
        context.type === 'selection'
          ? { type: 'quote', text: context.text }
          : { type: 'browser_annotation', input_json: JSON.stringify(context.browser_annotation ?? {}) },
      ),
      { type: 'text', text: live.user },
      ...live.attachments.flatMap<MessageBlock>((attachment) =>
        attachment.id && attachment.uri
          ? [{
              type: 'attachment',
              id: attachment.id,
              name: attachment.name,
              uri: attachment.uri,
              mime_type: attachment.mime_type,
              size: attachment.size,
              server_path: attachment.server_path,
            }]
          : [],
      ),
    ],
    created_at: live.at,
  }
}

function liveContextAttachments(contexts: ComposerContext[]): LiveAttachment[] {
  return contexts.flatMap((context) =>
    context.type === 'browser_annotation' && context.screenshotAttachment?.id
      ? [{
          id: context.screenshotAttachment.id,
          name: context.screenshotAttachment.name ?? 'annotation screenshot',
          uri: context.screenshotAttachment.uri,
          mime_type: context.screenshotAttachment.mime_type,
          size: context.screenshotAttachment.size,
          server_path: context.screenshotAttachment.server_path,
        }]
      : [],
  )
}

function mergeLiveStreamEvent(prev: LiveExchange, event: AgentStreamEvent): LiveExchange {
  switch (event.type) {
    case 'delta':
      return { ...prev, assistant: prev.assistant + (event.delta ?? '') }
    case 'reasoning':
      return { ...prev, reasoning: prev.reasoning + (event.reasoning ?? '') }
    case 'tool_call':
      return addLiveToolCall(prev, event)
    case 'tool_result':
      return addLiveToolResult(prev, event)
    case 'error':
      return {
        ...prev,
        attachments: finishLiveAttachments(prev.attachments),
        error: event.error || 'Something went wrong.',
      }
    default:
      return prev
  }
}

function addLiveToolCall(prev: LiveExchange, event: AgentStreamEvent): LiveExchange {
  const name = event.tool_call?.function?.name ?? event.tool_name ?? 'tool'
  if (isHiddenToolName(name)) return prev
  return {
    ...prev,
    tools: [
      ...prev.tools,
      {
        key: event.tool_call?.id ?? `${name}-${prev.tools.length}`,
        name,
        args: event.tool_call?.function?.arguments,
      },
    ],
  }
}

function addLiveToolResult(prev: LiveExchange, event: AgentStreamEvent): LiveExchange {
  if (isHiddenToolName(event.tool_name)) return prev
  const idx = prev.tools.findLastIndex((tool) => tool.result === undefined)
  const tools =
    idx === -1
      ? [
          ...prev.tools,
          {
            key: `result-${prev.tools.length}`,
            name: event.tool_name ?? 'tool',
            result: event.result,
          },
        ]
      : prev.tools.map((tool, index) => (index === idx ? { ...tool, result: event.result } : tool))
  return { ...prev, tools }
}

function finishLiveAttachments(attachments: LiveAttachment[]): LiveAttachment[] {
  return attachments.map((attachment) => ({ ...attachment, uploading: false }))
}
