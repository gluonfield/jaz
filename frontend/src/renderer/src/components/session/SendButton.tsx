import { type ReactNode, useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'

// The composer's primary action. With something to queue, hovering it (or
// pressing and holding on touch) offers Send and Queue with their keys. The
// portaled menu's pointer events reach the wrapper through React, so moving
// from the button onto the menu keeps it open.
export function SendButton({
  label,
  disabled,
  onSend,
  onQueue,
  children,
}: {
  label: string
  disabled: boolean
  onSend: () => void
  onQueue?: () => void
  children: ReactNode
}) {
  const [hovered, setHovered] = useState(false)
  const closeTimer = useRef<number | undefined>(undefined)
  const hold = useContextMenuTrigger(() => setHovered(true))
  const queue = disabled ? undefined : onQueue
  const triggers = queue ? hold : undefined
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
        open={hovered && Boolean(queue)}
        onClose={() => setHovered(false)}
        align="end"
        trigger={
          <IconButton
            variant="primary"
            size="md"
            className="relative after:absolute after:-inset-1"
            aria-label={label}
            title={queue ? undefined : label}
            disabled={disabled}
            {...triggers}
            onClick={(event) => {
              triggers?.onClick(event)
              if (!event.defaultPrevented) {
                onSend()
              }
            }}
          >
            {children}
          </IconButton>
        }
      >
        <MenuRow trailing={<KeyboardShortcut modifier="" value="↵" />} onClick={() => choose(onSend)}>
          Send
        </MenuRow>
        {queue ? (
          <MenuRow trailing={<KeyboardShortcut value="↵" />} onClick={() => choose(queue)}>
            Queue
          </MenuRow>
        ) : null}
      </Popover>
    </div>
  )
}
