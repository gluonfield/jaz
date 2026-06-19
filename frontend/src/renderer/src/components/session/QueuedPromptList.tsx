import { Check, CornerDownRight, GripVertical, Paperclip, Pencil, Trash2, X } from 'lucide-react'
import { AnimatePresence, motion, Reorder, useDragControls } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import type { QueuedMessage } from '@/lib/api/types'
import { MentionText } from './mentions'

export function QueuedPromptList({
  prompts,
  steerDisabled,
  onSteer,
  onDelete,
  onEdit,
  onReorder,
}: {
  prompts: QueuedMessage[]
  steerDisabled?: boolean
  onSteer: (id: string) => void
  onDelete: (id: string) => void
  onEdit: (id: string, text: string) => void
  onReorder: (ids: string[]) => void
}) {
  const [editingId, setEditingId] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  const [draggingId, setDraggingId] = useState<string | null>(null)

  const [items, setItems] = useState<QueuedMessage[]>(prompts)
  const syncedSig = useRef(queueSignature(prompts))
  const itemsRef = useRef(items)
  itemsRef.current = items

  useEffect(() => {
    const sig = queueSignature(prompts)
    if (sig === syncedSig.current) return
    syncedSig.current = sig
    setItems(prompts)
  }, [prompts])

  useEffect(() => {
    if (editingId !== null && !items.some((item) => item.id === editingId)) {
      setEditingId(null)
      setDraft('')
    }
  }, [editingId, items])

  const startEdit = (item: QueuedMessage) => {
    setEditingId(item.id)
    setDraft(item.text)
  }
  const finishEdit = () => {
    if (editingId === null) return
    const trimmed = draft.trim()
    if (trimmed) onEdit(editingId, trimmed)
    setEditingId(null)
    setDraft('')
  }
  const cancelEdit = () => {
    setEditingId(null)
    setDraft('')
  }

  if (items.length === 0) return null

  const dragDisabled = editingId !== null

  return (
    <motion.div
      layout="position"
      className="mb-2 overflow-hidden rounded-[12px] border border-border bg-surface shadow-sm"
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 8 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
    >
      <Reorder.Group axis="y" as="div" values={items} onReorder={setItems} className="flex flex-col py-1">
        <AnimatePresence initial={false} mode="popLayout">
          {items.map((item) => (
            <QueuedRow
              key={item.id}
              item={item}
              editing={editingId === item.id}
              dragDisabled={dragDisabled}
              dragging={draggingId === item.id}
              draft={draft}
              steerDisabled={steerDisabled}
              onDragStart={() => {
                setDraggingId(item.id)
              }}
              onDragEnd={() => {
                setDraggingId(null)
                const ids = itemsRef.current.map((prompt) => prompt.id)
                if (queueOrderSignature(prompts) !== ids.join('|')) onReorder(ids)
              }}
              onSteer={() => onSteer(item.id)}
              onDelete={() => onDelete(item.id)}
              onStartEdit={() => startEdit(item)}
              onDraftChange={setDraft}
              onSubmitEdit={finishEdit}
              onCancelEdit={cancelEdit}
            />
          ))}
        </AnimatePresence>
      </Reorder.Group>
    </motion.div>
  )
}

function QueuedRow({
  item,
  editing,
  dragDisabled,
  dragging,
  draft,
  steerDisabled,
  onDragStart,
  onDragEnd,
  onSteer,
  onDelete,
  onStartEdit,
  onDraftChange,
  onSubmitEdit,
  onCancelEdit,
}: {
  item: QueuedMessage
  editing: boolean
  dragDisabled: boolean
  dragging: boolean
  draft: string
  steerDisabled?: boolean
  onDragStart: () => void
  onDragEnd: () => void
  onSteer: () => void
  onDelete: () => void
  onStartEdit: () => void
  onDraftChange: (text: string) => void
  onSubmitEdit: () => void
  onCancelEdit: () => void
}) {
  // Drag is initiated only from the grip handle so the action buttons and selectable
  // text stay interactive (a row-wide drag listener would swallow their clicks).
  const controls = useDragControls()

  return (
    <Reorder.Item
      value={item}
      dragListener={false}
      dragControls={controls}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 8 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
      className={`grid min-h-8 grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-1.5 px-2 py-0.5 transition-colors duration-150 ${
        dragging ? 'bg-primary-soft/60' : 'hover:bg-surface-2'
      }`}
    >
      <button
        type="button"
        aria-label="Drag to reorder"
        title="Drag to reorder"
        disabled={dragDisabled}
        onPointerDown={(event) => {
          if (!dragDisabled) controls.start(event)
        }}
        className="grid h-7 w-5 touch-none cursor-grab place-items-center text-ink-3 transition-colors duration-150 hover:text-ink active:cursor-grabbing disabled:cursor-default disabled:opacity-40"
      >
        <GripVertical className="size-3.5" aria-hidden />
      </button>
      {editing ? (
        <input
          value={draft}
          autoFocus
          className="h-7 min-w-0 rounded-control bg-bg px-2 text-[13px] text-ink placeholder:text-ink-3"
          onChange={(event) => onDraftChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              onSubmitEdit()
            }
            if (event.key === 'Escape') {
              event.preventDefault()
              onCancelEdit()
            }
          }}
        />
      ) : (
        <div className="flex min-w-0 items-center gap-1.5">
          <CornerDownRight className="size-3.5 shrink-0 text-ink-3" aria-hidden />
          <p className="truncate text-[13px] text-ink-2 select-text">
            <MentionText text={item.text} />
          </p>
          {item.attachment_ids?.length ? (
            <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-bg px-1.5 py-0.5 text-[11px] tabular-nums text-ink-3">
              <Paperclip size={11} aria-hidden />
              {item.attachment_ids.length}
            </span>
          ) : null}
          {item.plan_requested ? (
            <span className="inline-flex shrink-0 rounded-full bg-primary-soft px-1.5 py-0.5 text-[11px] font-medium text-primary">
              Plan
            </span>
          ) : null}
        </div>
      )}
      <div className="flex items-center gap-1">
        {editing ? (
          <>
            <motion.button
              type="button"
              aria-label="Save queued prompt"
              title="Save queued prompt"
              disabled={!draft.trim()}
              onClick={onSubmitEdit}
              whileTap={{ scale: 0.92 }}
              className="grid size-6 cursor-pointer place-items-center rounded-full text-primary transition-colors duration-150 hover:bg-primary-soft disabled:cursor-default disabled:text-ink-3"
            >
              <Check size={14} />
            </motion.button>
            <motion.button
              type="button"
              aria-label="Cancel edit"
              title="Cancel edit"
              onClick={onCancelEdit}
              whileTap={{ scale: 0.92 }}
              className="grid size-6 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
            >
              <X size={14} />
            </motion.button>
          </>
        ) : (
          <>
            <motion.button
              type="button"
              disabled={steerDisabled}
              onClick={onSteer}
              whileTap={{ scale: 0.97 }}
              className="inline-flex h-6 cursor-pointer items-center gap-1 rounded-control px-1.5 text-[11px] font-medium text-ink-2 transition-colors duration-150 hover:bg-primary-soft hover:text-primary disabled:cursor-default disabled:opacity-45"
            >
              <CornerDownRight size={13} />
              Steer
            </motion.button>
            <motion.button
              type="button"
              aria-label="Edit queued prompt"
              title="Edit queued prompt"
              onClick={onStartEdit}
              whileTap={{ scale: 0.92 }}
              className="grid size-6 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
            >
              <Pencil size={14} />
            </motion.button>
            <motion.button
              type="button"
              aria-label="Delete queued prompt"
              title="Delete queued prompt"
              onClick={onDelete}
              whileTap={{ scale: 0.92 }}
              className="grid size-6 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-danger-soft hover:text-danger"
            >
              <Trash2 size={14} />
            </motion.button>
          </>
        )}
      </div>
    </Reorder.Item>
  )
}

function queueOrderSignature(prompts: QueuedMessage[]): string {
  return prompts.map((prompt) => prompt.id).join('|')
}

function queueSignature(prompts: QueuedMessage[]): string {
  return JSON.stringify(
    prompts.map((prompt) => [prompt.id, prompt.text, prompt.attachment_ids ?? [], prompt.plan_requested ?? false]),
  )
}
