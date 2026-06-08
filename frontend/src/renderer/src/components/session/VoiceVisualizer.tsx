import { useEffect, useRef } from 'react'

type Phase = 'connecting' | 'listening' | 'thinking' | 'speaking' | 'paused' | 'error'

const BARS = 72

function readPalette() {
  const s = getComputedStyle(document.documentElement)
  const rainbow = Array.from({ length: 5 }, (_, i) =>
    s.getPropertyValue(`--color-rainbow-${i + 1}`).trim(),
  )
  return { rainbow }
}

// Circular spectrum + breathing core, rendered on a canvas and driven by the
// live analyser of whatever is making sound (mic while listening, the assistant
// while speaking). When nothing is flowing it falls back to a calm idle pulse,
// so the orb always feels alive but only *reacts* to real audio.
export function VoiceVisualizer({
  analyser,
  phase,
  reducedMotion,
  size = 248,
}: {
  analyser: AnalyserNode | null
  phase: Phase
  reducedMotion: boolean | null
  size?: number
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const analyserRef = useRef(analyser)
  const phaseRef = useRef(phase)
  analyserRef.current = analyser
  phaseRef.current = phase

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    const dpr = window.devicePixelRatio || 1
    canvas.width = size * dpr
    canvas.height = size * dpr
    ctx.scale(dpr, dpr)

    let palette = readPalette()
    // re-read tokens if the theme flips while voice mode is open
    const observer = new MutationObserver(() => {
      palette = readPalette()
    })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })

    let freq = new Uint8Array(0)
    const amps = new Array(BARS).fill(0)
    const cx = size / 2
    const cy = size / 2
    const R = size * 0.3
    let rot = 0
    let level = 0
    let raf = 0

    const frame = (now: number) => {
      raf = requestAnimationFrame(frame)
      const a = analyserRef.current
      const p = phaseRef.current
      let target = 0

      if (a && (p === 'listening' || p === 'speaking')) {
        if (freq.length !== a.frequencyBinCount) freq = new Uint8Array(a.frequencyBinCount)
        a.getByteFrequencyData(freq)
        const usable = Math.floor(freq.length * 0.62) // voice band, skip dead highs
        let sum = 0
        for (let i = 0; i < BARS; i++) {
          const v = freq[Math.floor((i / BARS) * usable)] / 255
          amps[i] += (v - amps[i]) * 0.4
          sum += amps[i]
        }
        target = sum / BARS
      } else {
        // idle / thinking: gentle synthetic shimmer, no claim of input
        const breathe = reducedMotion ? 0.05 : 0.05 + 0.04 * Math.sin(now / 700)
        for (let i = 0; i < BARS; i++) {
          const v = reducedMotion ? 0.05 : breathe + 0.03 * Math.sin(now / 320 + i * 0.5)
          amps[i] += (v - amps[i]) * 0.2
        }
        target = breathe
      }
      level += (target - level) * 0.25

      const spin = reducedMotion ? 0 : p === 'thinking' ? 0.016 : p === 'listening' ? 0.004 : 0.002
      rot += spin

      ctx.clearRect(0, 0, size, size)
      ctx.save()
      ctx.translate(cx, cy)

      // glowing core
      const orbR = R * (0.74 + level * 0.55)
      const grad = ctx.createRadialGradient(0, 0, orbR * 0.15, 0, 0, orbR)
      grad.addColorStop(0, palette.rainbow[2])
      grad.addColorStop(0.55, palette.rainbow[3])
      grad.addColorStop(1, palette.rainbow[4])
      ctx.globalAlpha = 0.16 + level * 0.5
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.arc(0, 0, orbR, 0, Math.PI * 2)
      ctx.fill()
      ctx.globalAlpha = 1

      // spectrum corona
      ctx.lineCap = 'round'
      ctx.lineWidth = size * 0.012
      for (let i = 0; i < BARS; i++) {
        const ang = (i / BARS) * Math.PI * 2 + rot
        const len = R * 0.16 + amps[i] * R * 0.78
        const cos = Math.cos(ang)
        const sin = Math.sin(ang)
        ctx.strokeStyle = palette.rainbow[Math.floor((i / BARS) * 5) % 5]
        ctx.globalAlpha = 0.5 + amps[i] * 0.5
        ctx.beginPath()
        ctx.moveTo(cos * R, sin * R)
        ctx.lineTo(cos * (R + len), sin * (R + len))
        ctx.stroke()
      }
      ctx.restore()
    }
    raf = requestAnimationFrame(frame)

    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
    }
  }, [size, reducedMotion])

  return (
    <canvas
      ref={canvasRef}
      aria-hidden
      style={{ width: size, height: size }}
      className="pointer-events-none"
    />
  )
}
