import { FileText, Folder, Sparkles } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef } from 'react'
import { AgentAvatar } from '@/components/acp/AgentAvatar'
import { fullTime, hasTime, relativeTime } from '@/lib/format/time'

// One row in the composer's $/@ autocomplete. `insert` is the literal token
// text placed in the textarea; `expansion` is what that text becomes in the
// sent message (a skill reference, a thread id, an absolute path).
export interface SuggestionItem {
  kind: 'skill' | 'project' | 'thread' | 'file' | 'dir'
  label: string
  detail?: string
  /** label indices matched by the fuzzy query, for highlighting */
  indices?: number[]
  agent?: string
  updatedAt?: string
  insert: string
  expansion: string
}

// Sections keep the menu general: $ shows Skills, @ can show thread/path sources.
export interface SuggestionSection {
  title: string
  items: SuggestionItem[]
}

function ItemIcon({ item }: { item: SuggestionItem }) {
  if (item.kind === 'skill') return <Sparkles size={13} className="mt-0.5 shrink-0 text-primary" />
  if (item.kind === 'thread') return <AgentAvatar agent={item.agent} size={15} className="mt-0.5" />
  if (item.kind === 'project' || item.kind === 'dir') return <Folder size={13} className="mt-0.5 shrink-0 text-primary" />
  return <FileText size={13} className="mt-0.5 shrink-0 text-ink-3" />
}

// Label with fuzzy-matched characters accented, grouped into runs so the DOM
// stays one span per stretch rather than one per character.
function HighlightedLabel({ text, indices }: { text: string; indices?: number[] }) {
  if (!indices || indices.length === 0) return <>{text}</>
  const matched = new Set(indices)
  const runs: { text: string; hit: boolean }[] = []
  for (let i = 0; i < text.length; i++) {
    const hit = matched.has(i)
    const last = runs[runs.length - 1]
    if (last && last.hit === hit) last.text += text[i]
    else runs.push({ text: text[i], hit })
  }
  return (
    <>
      {runs.map((run, index) =>
        run.hit ? (
          <span key={index} className="font-medium text-primary">
            {run.text}
          </span>
        ) : (
          <span key={index}>{run.text}</span>
        ),
      )}
    </>
  )
}

// The floating panel above the composer card. The parent owns all state
// (sections, the active row, selection); `activeIndex` indexes the flattened
// items across sections. Rows select on mousedown so the textarea never blurs.
export function ComposerSuggestions({
  sections,
  activeIndex,
  onHover,
  onSelect,
}: {
  sections: SuggestionSection[]
  activeIndex: number
  onHover: (index: number) => void
  onSelect: (item: SuggestionItem) => void
}) {
  const reducedMotion = useReducedMotion()
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    listRef.current
      ?.querySelector('[data-active="true"]')
      ?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex])

  let index = -1
  return (
    <motion.div
      ref={listRef}
      role="listbox"
      data-escape-surface=""
      initial={{ opacity: 0, y: reducedMotion ? 0 : 6 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: reducedMotion ? 0 : 6 }}
      transition={{ duration: 0.15, ease: 'easeOut' }}
      className="max-h-[280px] overflow-y-auto rounded-[14px] bg-surface p-1.5 shadow-xl ring-1 ring-border [-webkit-app-region:no-drag]"
    >
      {sections.map((section) => (
        <div key={section.title}>
          <p className="px-2 pt-1 pb-0.5 text-[11px] text-ink-3">{section.title}</p>
          {section.items.map((item) => {
            index++
            const itemIndex = index
            const active = itemIndex === activeIndex
            const updatedAt = item.kind === 'thread' && hasTime(item.updatedAt) ? item.updatedAt : ''
            const updatedLabel = updatedAt ? relativeTime(updatedAt) : ''
            return (
              <button
                key={`${item.kind}-${item.insert}`}
                type="button"
                role="option"
                aria-selected={active}
                data-active={active}
                tabIndex={-1}
                // mousedown beats the textarea blur
                onMouseDown={(e) => {
                  e.preventDefault()
                  onSelect(item)
                }}
                onMouseEnter={() => onHover(itemIndex)}
                className={`flex w-full items-start gap-2 rounded-[8px] px-2 py-1.5 text-left transition-colors duration-150 ${
                  active ? 'bg-surface-2 text-ink' : 'text-ink-2'
                }`}
              >
                <ItemIcon item={item} />
                <span className="flex min-w-0 flex-1 items-baseline gap-2">
                  {/* the name never collapses in favor of its description —
                      flex may shrink overflow-hidden items to zero */}
                  <span
                    className={`truncate text-[13px] ${
                      item.detail ? 'max-w-[60%] shrink-0' : 'min-w-0'
                    }`}
                  >
                    <HighlightedLabel text={item.label} indices={item.indices} />
                  </span>
                  {item.detail ? (
                    <span className="min-w-0 flex-1 truncate text-[11px] text-ink-3">
                      {item.detail}
                    </span>
                  ) : null}
                </span>
                {updatedLabel ? (
                  <span
                    title={`Updated ${fullTime(updatedAt)}`}
                    className="ml-auto shrink-0 text-[11px] tabular-nums text-ink-3"
                  >
                    {updatedLabel}
                  </span>
                ) : null}
              </button>
            )
          })}
        </div>
      ))}
    </motion.div>
  )
}
