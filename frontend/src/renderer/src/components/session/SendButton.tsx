import { ListEnd } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'

// The composer's primary action. With something to queue, hovering it (or
// pressing and holding on touch) offers Send now and Queue with their keys,
// and holding ⌘ turns the button itself into Queue, as ⌘↵ does in the draft.
// The portaled menu's pointer events reach the wrapper through React, so
// moving from the button onto the menu keeps it open.
export function SendButton({
  label,
  shortcut,
  disabled,
  onSend,
  onQueue,
  children,
}: {
  label: string
  shortcut?: string
  disabled: boolean
  onSend: () => void
  onQueue?: () => void
  children: ReactNode
}) {
  const [hovered, setHovered] = useState(false)
  const queueing = useMetaHeld(Boolean(onQueue))
  const action = queueing && onQueue ? onQueue : onSend
  const closeTimer = useRef<number | undefined>(undefined)
  const hold = useContextMenuTrigger(() => setHovered(true))
  const triggers = onQueue ? hold : undefined
  useEffect(() => () => window.clearTimeout(closeTimer.current), [])

  const choose = (action: () => void) => {
    setHovered(false)
    action()
  }

  return (
    <div
      onPointerEnter={(event) => {
        window.clearTimeout(closeTimer.current)
        if (event.pointerType === 'mouse') {
          setHovered(true)
        }
      }}
      onPointerLeave={(event) => {
        if (event.pointerType === 'mouse') {
          closeTimer.current = window.setTimeout(() => setHovered(false), 150)
        }
      }}
    >
      <Popover
        open={hovered && Boolean(onQueue)}
        onClose={() => setHovered(false)}
        align="end"
        trigger={
          <IconButton
            variant="primary"
            size="md"
            className="relative after:absolute after:-inset-1"
            aria-label={label}
            aria-keyshortcuts={shortcut}
            title={onQueue ? undefined : label}
            disabled={disabled}
            {...triggers}
            onClick={(event) => {
              triggers?.onClick(event)
              if (!event.defaultPrevented) {
                action()
              }
            }}
          >
            <AnimatePresence initial={false}>
              <motion.span
                key={String(queueing)}
                className="[grid-area:1/1]"
                initial={{ opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
                animate={{ opacity: 1, scale: 1, filter: 'blur(0px)' }}
                exit={{ opacity: 0, scale: 0.25, filter: 'blur(4px)' }}
                transition={{ type: 'spring', duration: 0.3, bounce: 0 }}
              >
                {queueing ? <ListEnd size={16} /> : children}
              </motion.span>
            </AnimatePresence>
          </IconButton>
        }
      >
        <MenuRow trailing={<KeyboardShortcut modifier="" value="↵" />} onClick={() => choose(onSend)}>
          Send now
        </MenuRow>
        {onQueue ? (
          <MenuRow active={queueing} trailing={<KeyboardShortcut value="↵" />} onClick={() => choose(onQueue)}>
            Queue
          </MenuRow>
        ) : null}
      </Popover>
    </div>
  )
}
