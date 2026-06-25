import { Target, X } from 'lucide-react'
import { motion } from 'motion/react'
import { IconButton } from '@/components/ui/IconButton'

export function GoalMenuToggle({
  checked,
  disabled,
  onToggle,
}: {
  checked: boolean
  disabled?: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={onToggle}
      className={`flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-left text-[13px] transition-colors duration-150 enabled:hover:bg-surface-2 disabled:cursor-default disabled:opacity-50 ${
        checked ? 'text-ink' : 'text-ink-2'
      }`}
    >
      <Target size={13} className="shrink-0" />
      <span className="min-w-0 flex-1 truncate">Goal</span>
      <span
        aria-hidden
        className={`relative inline-flex h-4 w-7 shrink-0 items-center rounded-full transition-colors duration-150 ${
          checked ? 'bg-primary' : 'bg-ink/20'
        }`}
      >
        <motion.span
          layout
          transition={{ type: 'spring', stiffness: 500, damping: 34 }}
          className={`absolute size-3 rounded-full ${
            checked ? 'right-0.5 bg-on-primary' : 'left-0.5 bg-ink/60'
          }`}
        />
      </span>
    </button>
  )
}

export function GoalUnsupportedRow() {
  return (
    <div className="flex h-7 w-full items-center gap-2 rounded-full px-2.5 text-[13px] text-ink-3">
      <Target size={13} className="shrink-0" />
      <span className="min-w-0 flex-1 truncate">Goal</span>
      <span className="shrink-0 text-[12px]">Unsupported</span>
    </div>
  )
}

export function GoalChip({
  active,
  requested,
  disabled,
  onRemove,
}: {
  active: boolean
  requested: boolean
  disabled?: boolean
  onRemove: () => void
}) {
  return (
    <motion.div
      key="goal-chip"
      initial={{ opacity: 0, scale: 0.8, filter: 'blur(4px)' }}
      animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
      exit={{ opacity: 0, scale: 0.8, filter: 'blur(4px)' }}
      transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
      title={active ? 'Goal active' : undefined}
      className={`flex h-8 shrink-0 items-center gap-1 rounded-full pr-2.5 pl-1 text-[13px] font-medium text-ink-2 transition-colors duration-150 hover:bg-surface-2 hover:text-ink ${
        requested ? 'group' : ''
      }`}
    >
      {requested ? (
        <IconButton
          variant="ghost"
          size="xs"
          aria-label="Remove goal mode"
          title="Remove goal mode"
          disabled={disabled}
          className="grid"
          onClick={onRemove}
        >
          <Target
            size={13}
            className="col-start-1 row-start-1 transition-opacity group-hover:opacity-0 group-focus-within:opacity-0"
          />
          <X
            size={13}
            className="col-start-1 row-start-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
          />
        </IconButton>
      ) : (
        <span className="grid size-6 place-items-center" aria-hidden>
          <Target size={13} />
        </span>
      )}
      <span>Goal</span>
    </motion.div>
  )
}
