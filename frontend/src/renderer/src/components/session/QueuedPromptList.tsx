import { Check, CornerDownRight, GripVertical, Pencil, Trash2, X } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useState } from 'react'

export function QueuedPromptList({
  prompts,
  steerDisabled,
  onSteer,
  onDelete,
  onEdit,
  onMove,
}: {
  prompts: string[]
  steerDisabled?: boolean
  onSteer: (index: number) => void
  onDelete: (index: number) => void
  onEdit: (index: number, text: string) => void
  onMove: (from: number, to: number) => void
}) {
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [draft, setDraft] = useState('')
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)

  useEffect(() => {
    if (editingIndex !== null && editingIndex >= prompts.length) {
      setEditingIndex(null)
      setDraft('')
    }
  }, [editingIndex, prompts.length])

  const startEdit = (index: number) => {
    setEditingIndex(index)
    setDraft(prompts[index] ?? '')
  }
  const finishEdit = () => {
    if (editingIndex === null) return
    const trimmed = draft.trim()
    if (trimmed) onEdit(editingIndex, trimmed)
    setEditingIndex(null)
    setDraft('')
  }

  if (prompts.length === 0) return null

  return (
    <motion.div
      layout
      className="mb-2 overflow-hidden rounded-[12px] border border-border bg-surface shadow-sm"
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 8 }}
      transition={{ duration: 0.18, ease: 'easeOut' }}
    >
      <div className="flex flex-col py-1">
        {prompts.map((prompt, index) => {
          const editing = editingIndex === index
          return (
            <motion.div
              key={`${index}-${prompt}`}
              layout
              draggable={!editing}
              onDragStart={() => setDraggingIndex(index)}
              onDragEnd={() => setDraggingIndex(null)}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault()
                if (draggingIndex !== null && draggingIndex !== index) onMove(draggingIndex, index)
                setDraggingIndex(null)
              }}
              className={`grid min-h-10 grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-2 px-2.5 py-1 transition-colors duration-150 ${
                draggingIndex === index ? 'bg-primary-soft/60' : 'hover:bg-surface-2'
              }`}
            >
              <GripVertical className="size-4 cursor-grab text-ink-3 active:cursor-grabbing" aria-hidden />
              {editing ? (
                <input
                  value={draft}
                  autoFocus
                  className="h-8 min-w-0 rounded-control bg-bg px-2 text-sm text-ink placeholder:text-ink-3"
                  onChange={(event) => setDraft(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.preventDefault()
                      finishEdit()
                    }
                    if (event.key === 'Escape') {
                      event.preventDefault()
                      setEditingIndex(null)
                      setDraft('')
                    }
                  }}
                />
              ) : (
                <div className="flex min-w-0 items-center gap-2">
                  <CornerDownRight className="size-4 shrink-0 text-ink-3" aria-hidden />
                  <p className="truncate text-sm text-ink-2 select-text">{prompt}</p>
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
                      onClick={finishEdit}
                      whileTap={{ scale: 0.92 }}
                      className="grid size-7 cursor-pointer place-items-center rounded-full text-primary transition-colors duration-150 hover:bg-primary-soft disabled:cursor-default disabled:text-ink-3"
                    >
                      <Check size={15} />
                    </motion.button>
                    <motion.button
                      type="button"
                      aria-label="Cancel edit"
                      title="Cancel edit"
                      onClick={() => {
                        setEditingIndex(null)
                        setDraft('')
                      }}
                      whileTap={{ scale: 0.92 }}
                      className="grid size-7 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                    >
                      <X size={15} />
                    </motion.button>
                  </>
                ) : (
                  <>
                    <motion.button
                      type="button"
                      disabled={steerDisabled}
                      onClick={() => onSteer(index)}
                      whileTap={{ scale: 0.97 }}
                      className="inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-control px-2 text-[12px] font-medium text-ink-2 transition-colors duration-150 hover:bg-primary-soft hover:text-primary disabled:cursor-default disabled:opacity-45"
                    >
                      <CornerDownRight size={14} />
                      Steer
                    </motion.button>
                    <motion.button
                      type="button"
                      aria-label="Edit queued prompt"
                      title="Edit queued prompt"
                      onClick={() => startEdit(index)}
                      whileTap={{ scale: 0.92 }}
                      className="grid size-7 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-surface-2 hover:text-ink"
                    >
                      <Pencil size={14} />
                    </motion.button>
                    <motion.button
                      type="button"
                      aria-label="Delete queued prompt"
                      title="Delete queued prompt"
                      onClick={() => onDelete(index)}
                      whileTap={{ scale: 0.92 }}
                      className="grid size-7 cursor-pointer place-items-center rounded-full text-ink-3 transition-colors duration-150 hover:bg-danger-soft hover:text-danger"
                    >
                      <Trash2 size={14} />
                    </motion.button>
                  </>
                )}
              </div>
            </motion.div>
          )
        })}
      </div>
    </motion.div>
  )
}
