import { isCloudApiSession } from '@/lib/api/client'
import { createCloudTurn } from '@/lib/api/cloudThreads'
import { uploadCloudFile } from '@/lib/api/cloudUploads'
import {
  mutateSessionQueue,
  type QueueMutation,
  uploadSessionAttachment,
} from '@/lib/api/sessions'
import type { Attachment, QueuedMessageInput } from '@/lib/api/types'
import { normalizeQueuedMessageInput } from '@/lib/sessionQueue'
import { preparedSendMessage, type SendMessageOptions } from '@/lib/sendMessage'
import { telemetry } from '@/lib/telemetry'

type AppendMutation = Extract<QueueMutation, { op: 'append' }>
type Append = (mutation: AppendMutation) => Promise<void>
type PromptKind = 'initial' | 'queued'

export interface AppendedSessionPrompt {
  message: QueuedMessageInput
  attachments: Attachment[]
}

export interface AppendPromptCloudContext {
  isCloud?: boolean
  runId?: string
  modelProfileId?: string
  executorTarget?: string
}

export async function appendSessionPrompt(
  sessionId: string,
  text: string,
  options: SendMessageOptions,
  kind: PromptKind,
  append?: Append,
  cloud?: AppendPromptCloudContext,
): Promise<AppendedSessionPrompt | null> {
  const isCloud = isCloudApiSession() || Boolean(cloud?.isCloud) || Boolean(cloud?.modelProfileId)
  const runId = cloud?.runId || sessionId
  const uploaded = options.files?.length
    ? await Promise.all(
        options.files.map((file) =>
          isCloud ? uploadCloudFile(runId, file) : uploadSessionAttachment(sessionId, file),
        ),
      )
    : []
  const prepared = preparedSendMessage(options, uploaded)
  const prompt = normalizeQueuedMessageInput({
    text,
    contexts: prepared.contexts,
    attachment_ids: prepared.attachmentIds,
    plan_requested: options.planRequested,
    goal_requested: options.goalRequested,
  })
  if (!prompt) return null
  const attachments = [...(options.attachments ?? []), ...uploaded]
  if (isCloud) {
    await createCloudTurn(sessionId, {
      text: prompt.text,
      model_profile_id: cloud?.modelProfileId,
      executor_target: cloud?.executorTarget,
      uploads: attachments,
    })
  } else {
    const submit: Append = append ?? (async (mutation) => {
      await mutateSessionQueue(sessionId, mutation)
    })
    await submit({ op: 'append', message: prompt })
  }
  telemetry.messageSent({
    queued: kind === 'queued',
    planRequested: Boolean(prompt.plan_requested),
    goalRequested: Boolean(prompt.goal_requested),
    attachmentCount: prompt.attachment_ids?.length ?? 0,
  })
  return { message: prompt, attachments }
}
