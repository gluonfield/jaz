import { useNavigate, useRouterState } from '@tanstack/react-router'
import { VoiceControls, VoiceMode } from '@/components/session/VoiceMode'
import type { VoiceHandle } from '@/lib/voice/session'
import { useGlobalVoice } from '@/lib/voice/VoiceProvider'
import { clientRuntime } from '@/lib/clientRuntime'
import { useRef, useState } from 'react'

export function FloatingVoice({ voice, onReturn, level, reducedMotion }: {
  voice: VoiceHandle
  onReturn: () => void
  level?: number
  reducedMotion?: boolean
}) {
  const [offset, setOffset] = useState({ x: 0, y: 0 })
  const gesture = useRef<{ x: number; y: number; moved: boolean; origin: typeof offset } | null>(null)
  const finish = () => {
    clientRuntime.voiceOverlay?.drag(null)
    gesture.current = null
  }
  return (
    <div style={{ transform: `translate(${offset.x}px, ${offset.y}px)` }} aria-label="Floating voice conversation" className="flex w-[196px] flex-col items-center rounded-[28px] bg-bg/95 px-4 pt-2 pb-4 text-ink shadow-[0_6px_30px_rgba(0,0,0,0.14)]">
      <button
        type="button"
        aria-label="Return to voice chat"
        title="Open voice chat · Drag to move"
        className="touch-none cursor-grab active:cursor-grabbing [-webkit-app-region:no-drag]"
        onPointerDown={(event) => {
          if (event.button !== 0) {
            return
          }
          event.currentTarget.setPointerCapture(event.pointerId)
          gesture.current = { x: event.screenX, y: event.screenY, moved: false, origin: offset }
          clientRuntime.voiceOverlay?.drag({ x: event.screenX, y: event.screenY })
        }}
        onPointerMove={(event) => {
          const start = gesture.current
          if (!start) {
            return
          }
          const x = event.screenX - start.x
          const y = event.screenY - start.y
          start.moved ||= Math.hypot(x, y) > 4
          if (!start.moved) {
            return
          }
          if (clientRuntime.voiceOverlay) {
            clientRuntime.voiceOverlay.drag({ x: event.screenX, y: event.screenY })
          } else {
            setOffset({ x: start.origin.x + x, y: start.origin.y + y })
          }
        }}
        onPointerUp={() => {
          const clicked = gesture.current && !gesture.current.moved
          finish()
          if (clicked) {
            onReturn()
          }
        }}
        onPointerCancel={finish}
        onLostPointerCapture={finish}
        onClick={(event) => {
          if (event.detail === 0) {
            onReturn()
          }
        }}
      >
        <VoiceMode voice={voice} size={152} level={level} compact reducedMotion={reducedMotion} />
      </button>
      <div className="flex items-center gap-3">
        <VoiceControls voice={voice} floating />
      </div>
    </div>
  )
}

export function GlobalVoice() {
  const voice = useGlobalVoice()
  const navigate = useNavigate()
  const location = useRouterState({ select: (state) => state.location })
  if (clientRuntime.voiceOverlay || voice.phase === 'off' || !voice.sessionId || (location.pathname === `/sessions/${voice.sessionId}` && !location.search.settings)) {
    return null
  }
  return (
    <div className="fixed right-5 bottom-5 z-[80] max-sm:right-3 max-sm:bottom-3">
      <FloatingVoice voice={voice} onReturn={() => void navigate({ to: '/sessions/$sessionId', params: { sessionId: voice.sessionId! }, search: {} })} />
    </div>
  )
}
