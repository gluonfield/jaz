import { type HTMLMotionProps, motion } from 'motion/react'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'
type Size = 'sm' | 'md' | 'lg'

const base =
  'inline-flex shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-full font-medium transition-colors duration-150 disabled:cursor-default'

const sizes: Record<Size, string> = {
  sm: 'h-7 px-2.5 text-[12px]',
  md: 'h-8 px-3 text-[13px]',
  lg: 'h-9 px-4 text-[13px]',
}

const variants: Record<Variant, string> = {
  primary:
    'bg-primary text-on-primary hover:bg-primary-strong disabled:bg-surface-2 disabled:text-ink-3',
  secondary: 'text-ink-2 hover:bg-surface-2 hover:text-ink disabled:opacity-50',
  ghost: 'text-ink-2 hover:bg-surface-2 hover:text-ink disabled:opacity-50',
  danger: 'text-ink-2 hover:bg-danger-soft hover:text-danger disabled:opacity-50',
}

// Pressed/selected toggle look: a quiet persistent fill (no fill at rest, like Codex).
const activeClass = 'bg-ink/15 text-ink'

// The single text-button primitive. `active` renders the selected/toggled state
// (Plan toggle, segmented options); everything else is variant + size.
export function Button({
  variant = 'secondary',
  size = 'md',
  active = false,
  className = '',
  children,
  ...props
}: {
  variant?: Variant
  size?: Size
  active?: boolean
} & HTMLMotionProps<'button'>) {
  return (
    <motion.button
      type="button"
      whileTap={props.disabled ? undefined : { scale: 0.97 }}
      className={`${base} ${sizes[size]} ${active ? activeClass : variants[variant]} ${className}`}
      {...props}
    >
      {children}
    </motion.button>
  )
}
