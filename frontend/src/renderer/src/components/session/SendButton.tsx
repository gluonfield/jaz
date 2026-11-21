import { ArrowUp, ListEnd } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { IconButton } from '@/components/ui/IconButton'
import { IconSwap } from '@/components/ui/IconSwap'
import { KeyboardShortcut } from '@/components/ui/KeyboardShortcut'
import { MenuRow, Popover } from '@/components/ui/Popover'
import { useContextMenuTrigger } from '@/lib/hooks/useContextMenuTrigger'
import { useMetaHeld } from '@/lib/hooks/useMetaHeld'

// Sends the composer's draft. With something to queue, hovering it (or
// pressing and holding on touch) offers Send now and Queue with their keys,
// and holding ⌘ turns the button itself into Queue, as ⌘↵ does in the draft.
// The portaled menu's pointer events reach the wrapper through React, so
// moving from the button onto the menu keeps it open.
export function SendButton({
  disabled,
  onSend,
  onQueue,
}: {
  disabled: boolean
  onSend: () => void
  onQueue?: () => void
}) {
  const [hovered, setHovered] = useState(false)
  const queueing = useMetaHeld(Boolean(onQueue))
  const press = queueing && onQueue ? onQueue : onSend
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
            aria-label="Send message"
            aria-keyshortcuts="Enter"
            title={onQueue ? undefined : 'Send message'}
            disabled={disabled}
            {...triggers}
            onClick={(event) => {
              triggers?.onClick(event)
              if (!event.defaultPrevented) {
                press()
              }
            }}
          >
            <IconSwap id={String(queueing)}>{queueing ? <ListEnd size={16} /> : <ArrowUp size={16} />}</IconSwap>
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
