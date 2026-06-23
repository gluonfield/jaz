export interface BrowserAnnotation {
  url?: string
  frame?: string
  target?: string
  selector?: string
  path?: string
  node_position?: { x?: number; y?: number }
  viewport?: { width?: number; height?: number }
  requested_changes?: string
  comment?: string
  screenshot_attachment_id?: string
}

export interface ContextAttachment {
  id: string
  name?: string
  uri?: string
  mime_type?: string
  size?: number
  server_path?: string
}

export type ComposerContext =
  | {
      id: string
      type: 'selection'
      text: string
    }
  | {
      id: string
      type: 'browser_annotation'
      browser_annotation: BrowserAnnotation
      screenshotAttachment?: ContextAttachment
    }

export type MessageContextInput =
  | { type: 'selection'; text: string }
  | { type: 'browser_annotation'; browser_annotation: BrowserAnnotation }

export function contextInputs(contexts: ComposerContext[] = []): MessageContextInput[] {
  return contexts.flatMap<MessageContextInput>((context) => {
    if (context.type === 'selection') {
      const text = context.text.trim()
      return text ? [{ type: 'selection' as const, text }] : []
    }
    const annotation = normalizeBrowserAnnotation(context.browser_annotation)
    if (!annotation) return []
    const screenshotAttachmentID = context.screenshotAttachment?.id ?? annotation.screenshot_attachment_id
    return [
      {
        type: 'browser_annotation' as const,
        browser_annotation: {
          ...annotation,
          ...(screenshotAttachmentID ? { screenshot_attachment_id: screenshotAttachmentID } : {}),
        },
      },
    ]
  })
}

export function contextAttachmentIDs(contexts: ComposerContext[] = []): string[] {
  return contexts.flatMap((context) => {
    if (context.type !== 'browser_annotation') return []
    const annotation = normalizeBrowserAnnotation(context.browser_annotation)
    if (!annotation) return []
    const id = context.screenshotAttachment?.id ?? annotation.screenshot_attachment_id
    return id ? [id] : []
  })
}

export function contextLabel(context: ComposerContext): string {
  return context.type === 'selection' ? 'Selection' : 'Annotation'
}

export function contextPreviewText(context: ComposerContext): string {
  if (context.type === 'selection') return context.text
  const annotation = context.browser_annotation
  return [annotation.target, annotation.comment || annotation.requested_changes].filter(Boolean).join('\n\n')
}

export function browserAnnotationFromJSON(raw?: string): BrowserAnnotation | null {
  if (!raw) return null
  try {
    return browserAnnotationFromUnknown(JSON.parse(raw))
  } catch {
    return null
  }
}

export function browserAnnotationFromUnknown(value: unknown): BrowserAnnotation | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  const annotation: BrowserAnnotation = {
    url: stringField(raw.url),
    frame: stringField(raw.frame),
    target: stringField(raw.target),
    selector: stringField(raw.selector),
    path: stringField(raw.path),
    requested_changes: stringField(raw.requested_changes),
    comment: stringField(raw.comment),
    screenshot_attachment_id: stringField(raw.screenshot_attachment_id),
  }
  const nodePosition = pointField(raw.node_position)
  if (nodePosition) annotation.node_position = nodePosition
  const viewport = viewportField(raw.viewport)
  if (viewport) annotation.viewport = viewport
  return normalizeBrowserAnnotation(annotation)
}

export function normalizeBrowserAnnotation(annotation: BrowserAnnotation | null | undefined): BrowserAnnotation | null {
  if (!annotation) return null
  const normalized: BrowserAnnotation = {
    url: stringField(annotation.url),
    frame: stringField(annotation.frame),
    target: stringField(annotation.target),
    selector: stringField(annotation.selector),
    path: stringField(annotation.path),
    requested_changes: stringField(annotation.requested_changes),
    comment: stringField(annotation.comment),
    screenshot_attachment_id: stringField(annotation.screenshot_attachment_id),
  }
  const nodePosition = pointField(annotation.node_position)
  if (nodePosition) normalized.node_position = nodePosition
  const viewport = viewportField(annotation.viewport)
  if (viewport) normalized.viewport = viewport
  return normalized.url || normalized.target || normalized.selector || normalized.comment || normalized.requested_changes
    ? normalized
    : null
}

function stringField(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function pointField(value: unknown): { x?: number; y?: number } | undefined {
  if (!value || typeof value !== 'object') return undefined
  const raw = value as Record<string, unknown>
  const point = { x: numberField(raw.x), y: numberField(raw.y) }
  return point.x === undefined && point.y === undefined ? undefined : point
}

function viewportField(value: unknown): { width?: number; height?: number } | undefined {
  if (!value || typeof value !== 'object') return undefined
  const raw = value as Record<string, unknown>
  const viewport = { width: numberField(raw.width), height: numberField(raw.height) }
  return viewport.width === undefined && viewport.height === undefined ? undefined : viewport
}

function numberField(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}
