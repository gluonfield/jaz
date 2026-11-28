import type { QueryClient } from '@tanstack/react-query'
import { createCloudTurn } from '@/lib/api/cloudThreads'
import { uploadCloudFile } from '@/lib/api/cloudUploads'
import { uploadSessionAttachment } from '@/lib/api/sessions'
import { streamSessionMessage, type AgentStreamEvent } from '@/lib/api/stream'
import type { Attachment, SessionMessages } from '@/lib/api/types'
import { userInputMessageBlocks } from '@/lib/messageBlocks'
import {
  outboxTranscript,
  patchOutboxMessage,
  putOutboxMessage,
  removeOutboxMessages,
  type OutboxChatMessage,
  type OutboxSend,
} from '@/lib/outbox'
import { keys } from '@/lib/query/keys'
import { contextInputs, preparedSendMessage, type ComposerContext, type SendMessageOptions } from '@/lib/sendMessage'

export function newOutboxSend(text: string, options: SendMessageOptions, baselineMessageSeq: number): OutboxSend {
  return {
    id: globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`,
    state: 'sending',
    baselineMessageSeq,
    text,
    options,
    uploaded: [],
    message: sentMessage(text, options, [], new Date().toISOString()),
  }
}

function sentMessage(text: string, options: SendMessageOptions, uploaded: Attachment[], at: string): OutboxChatMessage {
  const contexts = options.contexts ?? []
  return {
    role: 'user',
    content: text,
    created_at: at,
    blocks: userInputMessageBlocks(text, contextInputs(contexts), [
      ...(options.attachments ?? []),
      ...contextAttachments(contexts),
      ...uploaded,
    ]),
  }
}

function contextAttachments(contexts: ComposerContext[]): Attachment[] {
  return contexts.flatMap((context) =>
    context.type === 'browser_annotation' && context.screenshotAttachment?.id
      ? [{ ...context.screenshotAttachment, name: context.screenshotAttachment.name ?? 'annotation screenshot' }]
      : [],
  )
}

// Sends an outbox message and keeps its entry true to what the server said:
// sent once accepted, failed if it never was. `accepted` tells which; `finished`
// settles as the request does; `settled` follows the history reload, after
// which an accepted message is delivered: it leaves the outbox when its row is
// loaded.
// A Cloud thread names the run its uploads belong to.
export function deliverMessage({
  queryClient,
  sessionId,
  entry,
  cloudRunId,
  signal,
  onEvent,
}: {
  queryClient: QueryClient
  sessionId: string
  entry: OutboxSend
  cloudRunId?: string
  signal: AbortSignal
  onEvent: (event: AgentStreamEvent) => void
}): { accepted: Promise<boolean>; finished: Promise<void>; settled: Promise<void> } {
  const { id, text, options } = entry
  putOutboxMessage(sessionId, { ...entry, state: 'sending', error: undefined })
  const started = (async () => {
    let uploaded = entry.uploaded
    const files = options.files ?? []
    if (files.length) {
      uploaded = [...uploaded, ...await Promise.all(files.map((file) =>
        cloudRunId ? uploadCloudFile(cloudRunId, file, { signal }) : uploadSessionAttachment(sessionId, file, signal),
      ))]
      // A retry sends what is already uploaded.
      const sent = { ...options, files: [] }
      patchOutboxMessage(sessionId, id, {
        options: sent,
        uploaded,
        message: sentMessage(text, sent, uploaded, entry.message.created_at),
      })
    }
    if (cloudRunId) {
      // The outbox id makes a retry of a turn the server did take a replay.
      await createCloudTurn(sessionId, {
        text,
        client_request_id: id,
        uploads: [...(options.attachments ?? []), ...uploaded],
        signal,
      })
      // Keep the sent message in the outbox until its saved row is loaded:
      // Cloud turns may still be queueing or syncing when the first refetch runs.
      return { accepted: Promise.resolve(), finished: Promise.resolve(), saved: false }
    }
    const prepared = preparedSendMessage(options, uploaded)
    return {
      ...streamSessionMessage({
        sessionId,
        message: text,
        contexts: prepared.contexts,
        attachmentIds: prepared.attachmentIds,
        planRequested: options.planRequested,
        goalRequested: options.goalRequested,
        signal,
        onEvent,
      }),
      saved: true,
    }
  })()
  const finished = started.then((stream) => stream.finished)
  const accepted = started.then((stream) => stream.accepted).then(
    () => {
      patchOutboxMessage(sessionId, id, { state: 'sent' })
      return true
    },
    (error: Error) => {
      patchOutboxMessage(sessionId, id, { state: 'failed', error: signal.aborted ? undefined : error.message })
      return false
    },
  )
  const settled = finished.catch(() => undefined).then(async () => {
    const queryKey = keys.sessionMessages(sessionId)
    const reloaded = await queryClient.refetchQueries({ queryKey }, { throwOnError: true }).then(
      () => queryClient.getQueryState(queryKey)?.fetchStatus !== 'paused',
      () => false,
    )
    if (!(await accepted)) return
    const delivered: OutboxSend = { ...entry, state: 'delivered' }
    const messages = queryClient.getQueryData<SessionMessages>(queryKey)?.messages ?? []
    // The server saved it before accepting, yet history loaded since holds no
    // row for it: it keeps none, as for a command, so nothing is left to wait for.
    if ((await started).saved && reloaded && !outboxTranscript(messages, [delivered]).settled.length) {
      removeOutboxMessages(sessionId, [id])
    } else {
      patchOutboxMessage(sessionId, id, { state: 'delivered' })
    }
  })
  return { accepted, finished, settled }
}
