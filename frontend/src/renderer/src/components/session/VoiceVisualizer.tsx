import { useEffect, useRef, useState } from 'react'
import { motion, useMotionValue, useSpring } from 'motion/react'
import { shapeById, type ShapeId } from '@/lib/vendor/mote/shapes'
import { eyeById, type EyeId } from '@/lib/vendor/mote/eyes'
import { audioLevel } from '@/lib/voice/audioLevel'
import { VoiceAvatarActivity, type VoiceAvatarState } from '@/lib/voice/avatar'
import type { VoiceState } from '@/lib/voice/session'

const poses: Record<VoiceAvatarState, { shape: ShapeId; eyes: EyeId; tilt: number; gaze: number; duration: number }> = {
  listening: { shape: 'orb', eyes: 'listening', tilt: -3, gaze: 3, duration: 5.6 },
  thinking: { shape: 'cloud', eyes: 'thinking', tilt: -5, gaze: -7, duration: 4.8 },
  working: { shape: 'squircle', eyes: 'focused', tilt: 2, gaze: 9, duration: 3.2 },
  speaking: { shape: 'blob', eyes: 'joyful', tilt: 3, gaze: 3, duration: 2.8 },
  connecting: { shape: 'egg', eyes: 'searching', tilt: -4, gaze: 8, duration: 3.6 },
  muted: { shape: 'pebble', eyes: 'sleepy', tilt: 0, gaze: 0, duration: 7 },
  error: { shape: 'pebble', eyes: 'uneasy', tilt: -4, gaze: 0, duration: 7 },
}

export function VoiceVisualizer({ voice, reducedMotion, level = 0, outputLevel = 0, size = 88 }: {
  voice: VoiceState
  reducedMotion: boolean
  level?: number
  outputLevel?: number
  size?: number
}) {
  const input = useRef({ voice, level, outputLevel })
  const [meter] = useState(() => new VoiceAvatarActivity())
  const [state, setState] = useState(() => meter.sample(voice, level, outputLevel, performance.now()))
  const amplitude = useSpring(1, { stiffness: 280, damping: 26 })
  const lift = useSpring(0, { stiffness: 160, damping: 24 })
  const rotation = useSpring(0, { stiffness: 160, damping: 24 })
  const gazeX = useSpring(0, { stiffness: 160, damping: 24 })
  const gazeY = useSpring(0, { stiffness: 160, damping: 24 })
  const leftBlink = useMotionValue(1)
  const rightBlink = useMotionValue(1)
  const pose = poses[state]

  useEffect(() => {
    input.current = { voice, level, outputLevel }
  }, [voice, level, outputLevel])

  useEffect(() => {
    let raf = 0
    let timer: ReturnType<typeof setTimeout>
    let samples = new Uint8Array(0)
    let outputSamples = new Uint8Array(0)
    let previous: VoiceAvatarState | undefined
    let lastFrame = performance.now()
    let cycle = 0
    let blinkAt = lastFrame + 2200 + Math.random() * 1800
    let wink = false
    let winkUntil = 0
    const draw = () => {
      const now = performance.now()
      const { voice, level, outputLevel } = input.current
      if (voice.analyser && samples.length !== voice.analyser.fftSize) {
        samples = new Uint8Array(voice.analyser.fftSize)
      }
      if (voice.outputAnalyser && outputSamples.length !== voice.outputAnalyser.fftSize) {
        outputSamples = new Uint8Array(voice.outputAnalyser.fftSize)
      }
      const incoming = voice.muted ? 0 : voice.analyser ? audioLevel(voice.analyser, samples) : level
      const outgoing = voice.outputAnalyser ? audioLevel(voice.outputAnalyser, outputSamples) : outputLevel
      const next = meter.sample(voice, incoming, outgoing, now)
      const pose = poses[next]
      const still = reducedMotion || next === 'muted' || next === 'error'
      setState(next)
      if (!still) {
        cycle += Math.min(now - lastFrame, 64) / (pose.duration * 1000) * Math.PI * 2
      }
      lastFrame = now
      for (const [value, target] of [
        [amplitude, still ? 1 : 1 + (next === 'speaking' ? outgoing : incoming) * 0.12],
        [lift, still ? 0 : Math.cos(cycle) * 3 - 3],
        [rotation, still ? pose.tilt : pose.tilt * Math.cos(cycle)],
        [gazeX, still ? 0 : pose.gaze * Math.sin(cycle)],
        [gazeY, !still && next === 'thinking' ? -5 : 0],
      ] as const) {
        if (reducedMotion) {
          value.jump(target)
        } else {
          value.set(target)
        }
      }
      const acknowledged = previous && ['connecting', 'thinking', 'working'].includes(previous) && (next === 'listening' || next === 'speaking')
      if (!still && acknowledged && now > winkUntil) {
        wink = true
        blinkAt = now + 500
        winkUntil = blinkAt + 6000
      }
      previous = next
      if (now > blinkAt + (wink ? 480 : 280)) {
        blinkAt = now + 3200 + Math.random() * 2400
        wink = false
      }
      const progress = Math.max(0, Math.min(1, (now - blinkAt) / (wink ? 480 : 280)))
      const eyelid = 1 - 0.94 * Math.sin(Math.PI * progress)
      leftBlink.set(still || wink ? 1 : eyelid)
      rightBlink.set(still ? 1 : eyelid)
      if (reducedMotion) {
        timer = setTimeout(draw, 100)
      } else {
        raf = requestAnimationFrame(draw)
      }
    }
    draw()
    return () => {
      cancelAnimationFrame(raf)
      clearTimeout(timer)
    }
  }, [amplitude, gazeX, gazeY, leftBlink, lift, meter, reducedMotion, rightBlink, rotation])

  const eyes = eyeById(pose.eyes)
  const morph = { type: 'spring', duration: reducedMotion ? 0 : 0.55, bounce: 0 } as const
  return (
    <svg width={size} height={size} viewBox="0 0 360 360" role="img" aria-label={`Voice ${state}`} data-avatar="mote" data-state={state} data-shape={pose.shape}>
      <g transform="translate(20 20)">
        <motion.g style={{ scale: amplitude, transformOrigin: '160px 160px' }}>
          <motion.g style={{ y: lift, rotate: rotation, transformOrigin: '160px 160px' }}>
            <motion.path data-body fill="#2f8de3" initial={false} animate={{ d: shapeById(pose.shape).path }} transition={morph} />
            <motion.g style={{ x: gazeX, y: gazeY }}>
              <motion.g style={{ scaleY: leftBlink, transformOrigin: `${eyes.leftCenter.x}px ${eyes.leftCenter.y}px` }}>
                <motion.path data-eye="left" fill="#f4f2eb" initial={false} animate={{ d: eyes.leftPath }} transition={morph} />
              </motion.g>
              <motion.g style={{ scaleY: rightBlink, transformOrigin: `${eyes.rightCenter.x}px ${eyes.rightCenter.y}px` }}>
                <motion.path data-eye="right" fill="#f4f2eb" initial={false} animate={{ d: eyes.rightPath }} transition={morph} />
              </motion.g>
            </motion.g>
          </motion.g>
        </motion.g>
      </g>
    </svg>
  )
}
