import { type HTMLMotionProps, motion } from 'motion/react'

type Variant = 'ghost' | 'primary' | 'danger'
type Size = 'xs' | 'sm' | 'md' | 'lg'

// Heights mirror the text Button scale (sm h-7 / md h-8 / lg h-9) so icon and
// text buttons line up in a toolbar; xs is for compact in-row icon actions.
const sizes: Record<Size, string> = {
  xs: 'size-6',
  sm: 'size-7',
  md: 'size-8',
  lg: 'size-9',
}

const variants: Record<Variant, string> = {
  ghost: 'text-ink-3 hover:bg-surface-2 hover:text-ink disabled:opacity-50',
  primary:
    'bg-primary text-on-primary shadow-sm hover:bg-primary-strong disabled:bg-surface-2 disabled:text-ink-3 disabled:shadow-none',
  danger: 'text-ink-3 hover:bg-danger-soft hover:text-danger disabled:opacity-50',
}

// Square/round icon-only button. `round` is for the composer's circular send /
// voice controls; everything else uses the 8px control radius.
export function IconButton({
  variant = 'ghost',
  size = 'md',
  round = false,
  className = '',
  children,
  ...props
}: {
  variant?: Variant
  size?: Size
  round?: boolean
} & HTMLMotionProps<'button'>) {
  return (
    <motion.button
      type="button"
      whileTap={props.disabled ? undefined : { scale: 0.92 }}
      className={`grid shrink-0 cursor-pointer place-items-center transition-colors duration-150 disabled:cursor-default ${
        round ? 'rounded-full' : 'rounded-control'
      } ${sizes[size]} ${variants[variant]} ${className}`}
      {...props}
    >
      {children}
    </motion.button>
  )
}
