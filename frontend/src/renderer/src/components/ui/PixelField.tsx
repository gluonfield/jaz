import { useReducedMotion } from 'motion/react'
import { useEffect, useRef } from 'react'

const TAU = Math.PI * 2

// A CSS mask keeps a soft clearing around the centered heading + composer; the
// JS constants mirror it so constructions only form where the field is visible.
const MASK = { rx: 420, ry: 240, cx: 0.5, cy: 0.45 }
const MASK_CSS = `radial-gradient(ellipse ${MASK.rx}px ${MASK.ry}px at 50% 45%, transparent 42%, black 85%)`

const N_MAX = 24_000

const rand = (a: number, b: number) => a + Math.random() * (b - a)

/* ---------------- shape sources ----------------
 * A generator fills `out` with unit-space points (xy roughly in [-1,1], z is
 * shallow depth for parallax). Density does the drawing: at ~15-24k particles
 * a sampled glyph reads as solid, not an outline. */

type ShapeDef = {
  gen: (out: Float32Array, n: number) => void
  spin: boolean // 3D forms spin fully; pictograms only wobble so they stay readable
  scaleMul: number
  alpha: number
}

const sphere = (out: Float32Array, n: number) => {
  // fibonacci shell with a little radial fuzz
  const golden = Math.PI * (3 - Math.sqrt(5))
  for (let i = 0; i < n; i++) {
    const y = 1 - (i / (n - 1)) * 2
    const r = Math.sqrt(1 - y * y) * rand(0.94, 1)
    const a = golden * i
    out[i * 3] = Math.cos(a) * r
    out[i * 3 + 1] = y * rand(0.94, 1)
    out[i * 3 + 2] = Math.sin(a) * r * 0.5
  }
}

const torus = (out: Float32Array, n: number) => {
  for (let i = 0; i < n; i++) {
    const u = rand(0, TAU)
    const v = rand(0, TAU)
    const ring = 0.68 + 0.26 * Math.cos(v)
    out[i * 3] = Math.cos(u) * ring
    out[i * 3 + 1] = Math.sin(u) * ring
    out[i * 3 + 2] = (0.26 * Math.sin(v)) / 0.94
  }
}

const helix = (out: Float32Array, n: number) => {
  for (let i = 0; i < n; i++) {
    const strand = i % 2
    const t = rand(-1, 1)
    const a = t * Math.PI * 2.6 + strand * Math.PI
    out[i * 3] = Math.cos(a) * 0.55 + rand(-0.03, 0.03)
    out[i * 3 + 1] = t + rand(-0.02, 0.02)
    out[i * 3 + 2] = Math.sin(a) * 0.55 * 0.6
  }
}

/* Stroke path data lifted from lucide (already a dependency), 24x24 viewBox;
 * circles/lines pre-converted to path arcs/segments. */
const GLYPHS: Record<string, string[]> = {
  sparkles: [
    'M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z',
    'M20 2v4',
    'M22 4h-4',
    'M2 20a2 2 0 1 0 4 0a2 2 0 1 0-4 0',
  ],
  sun: [
    'M8 12a4 4 0 1 0 8 0a4 4 0 1 0-8 0',
    'M12 2v2',
    'M12 20v2',
    'm4.93 4.93 1.41 1.41',
    'm17.66 17.66 1.41 1.41',
    'M2 12h2',
    'M20 12h2',
    'm6.34 17.66-1.41 1.41',
    'm19.07 4.93-1.41 1.41',
  ],
  moon: [
    'M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401',
  ],
  music: [
    'M9 18V5l12-2v13',
    'M3 18a3 3 0 1 0 6 0a3 3 0 1 0-6 0',
    'M15 16a3 3 0 1 0 6 0a3 3 0 1 0-6 0',
  ],
  plane: [
    'M14.536 21.686a.5.5 0 0 0 .937-.024l6.5-19a.496.496 0 0 0-.635-.635l-19 6.5a.5.5 0 0 0-.024.937l7.93 3.18a2 2 0 0 1 1.112 1.11z',
    'm21.854 2.147-10.94 10.939',
  ],
  bulb: [
    'M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5',
    'M9 18h6',
    'M10 22h4',
  ],
  leaf: [
    'M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19 2c1 2 2 4.18 2 8 0 5.5-4.78 10-10 10Z',
    'M2 21c0-3 1.85-5.36 5.08-6C9.5 14.52 12 13 13 12',
  ],
  heart: [
    'M2 9.5a5.5 5.5 0 0 1 9.591-3.676.56.56 0 0 0 .818 0A5.49 5.49 0 0 1 22 9.5c0 2.29-1.5 4-3 5.5l-5.492 5.313a2 2 0 0 1-3 .019L5 15c-1.5-1.5-3-3.2-3-5.5',
  ],
  coffee: [
    'M10 2v2',
    'M14 2v2',
    'M16 8a1 1 0 0 1 1 1v8a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1h14a4 4 0 1 1 0 8h-1',
    'M6 2v2',
  ],
  rocket: [
    'M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5',
    'M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09',
    'M9 12a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.4 22.4 0 0 1-4 2z',
    'M9 12H4s.55-3.03 2-4c1.62-1.08 5 .05 5 .05',
  ],
  umbrella: [
    'M12 13v7a2 2 0 0 0 4 0',
    'M12 2v2',
    'M20.992 13a1 1 0 0 0 .97-1.274 10.284 10.284 0 0 0-19.923 0A1 1 0 0 0 3 13z',
  ],
  bird: [
    'M16 7h.01',
    'M3.4 18H12a8 8 0 0 0 8-8V7a4 4 0 0 0-7.28-2.3L2 20',
    'm20 7 2 .5-2 .5',
    'M10 18v3',
    'M14 17.75V21',
    'M7 18a6 6 0 0 0 3.84-10.61',
  ],
  star: [
    'M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z',
  ],
  book: [
    'M12 7v14',
    'M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z',
  ],
}

// Rasterize stroked paths offscreen and scatter particles uniformly over the
// inked pixels (the enchanted-twin image trick, but for vector paths). With
// thousands of samples the stroke reads as a solid form; shallow z gives the
// wobble some parallax.
const glyphGen =
  (paths: string[]) =>
  (out: Float32Array, n: number) => {
    const S = 200
    const pad = 24
    const cv = document.createElement('canvas')
    cv.width = cv.height = S
    const g = cv.getContext('2d')
    if (!g) return sphere(out, n)
    g.translate(pad, pad)
    g.scale((S - pad * 2) / 24, (S - pad * 2) / 24)
    g.lineWidth = 2.2
    g.lineCap = 'round'
    g.lineJoin = 'round'
    for (const d of paths) g.stroke(new Path2D(d))

    const data = g.getImageData(0, 0, S, S).data
    const inked: number[] = []
    for (let i = 3; i < data.length; i += 4) if (data[i] > 90) inked.push((i - 3) / 4)
    if (!inked.length) return sphere(out, n)

    const half = S / 2
    for (let i = 0; i < n; i++) {
      const px = inked[Math.floor(rand(0, inked.length))]
      out[i * 3] = (((px % S) + rand(-0.6, 0.6)) - half) / (half - pad / 2)
      out[i * 3 + 1] = -((Math.floor(px / S) + rand(-0.6, 0.6)) - half) / (half - pad / 2)
      out[i * 3 + 2] = rand(-0.07, 0.07)
    }
  }

const glyphDef = (name: keyof typeof GLYPHS, scaleMul = 1.15): ShapeDef => ({
  gen: glyphGen(GLYPHS[name]),
  spin: false,
  scaleMul,
  alpha: 0.6,
})

const SHAPES: Record<string, ShapeDef> = {
  sphere: { gen: sphere, spin: true, scaleMul: 0.9, alpha: 0.5 },
  torus: { gen: torus, spin: true, scaleMul: 0.95, alpha: 0.5 },
  helix: { gen: helix, spin: true, scaleMul: 0.95, alpha: 0.55 },
  sparkles: glyphDef('sparkles'),
  sun: glyphDef('sun'),
  moon: glyphDef('moon', 1.05),
  music: glyphDef('music'),
  plane: glyphDef('plane'),
  bulb: glyphDef('bulb'),
  leaf: glyphDef('leaf'),
  heart: glyphDef('heart'),
  coffee: glyphDef('coffee'),
  rocket: glyphDef('rocket'),
  umbrella: glyphDef('umbrella'),
  bird: glyphDef('bird'),
  star: glyphDef('star'),
  book: glyphDef('book'),
}

/* ---------------- shaders ----------------
 * Every particle owns a `from` and `to` position, each in its own frame
 * (center px / scale px / Y-rotation). Morphs ease per particle with a
 * staggered cubic, swirling gently around the destination mid-flight. */

const VERT = `#version 300 es
precision highp float;
in vec3 aFrom;
in vec3 aTo;
in vec4 aSeed; // stagger, phase, freq, size jitter
in vec3 aColor;
uniform vec2 uRes;
uniform float uDpr;
uniform float uTime;
uniform float uProgress;
uniform float uSwirl;
uniform float uAlphaMul;
uniform float uAlphaFrom;
uniform float uAlphaTo;
uniform vec2 uCenterFrom;
uniform vec2 uCenterTo;
uniform float uScaleFrom;
uniform float uScaleTo;
uniform float uRotFrom;
uniform float uRotTo;
out vec3 vColor;
out float vAlpha;

vec3 spinY(vec3 p, float a) {
  float c = cos(a), s = sin(a);
  return vec3(p.x * c + p.z * s, p.y, -p.x * s + p.z * c);
}

void main() {
  float span = 0.55; // stagger window of the morph
  float t = clamp((uProgress * (1.0 + span) - aSeed.x * span), 0.0, 1.0);
  float e = t * t * (3.0 - 2.0 * t);

  vec3 pf = spinY(aFrom, uRotFrom);
  vec3 pt = spinY(aTo, uRotTo);
  // shallow fake perspective: nearer points sit wider and larger
  vec2 xyF = uCenterFrom + pf.xy * uScaleFrom * (1.0 + pf.z * 0.35) * vec2(1.0, -1.0);
  vec2 xyT = uCenterTo + pt.xy * uScaleTo * (1.0 + pt.z * 0.35) * vec2(1.0, -1.0);
  vec2 xy = mix(xyF, xyT, e);

  // swirl peaks mid-flight, vanishes at both ends
  float swirl = uSwirl * e * (1.0 - e) * 2.2;
  if (swirl > 0.001) {
    vec2 rel = xy - uCenterTo;
    float c = cos(swirl), s = sin(swirl);
    xy = uCenterTo + vec2(rel.x * c - rel.y * s, rel.x * s + rel.y * c);
  }

  // ambient micro-drift, calmer while formed
  float settle = 1.0 - 0.55 * e * uAlphaTo;
  xy += vec2(
    sin(uTime * aSeed.z + aSeed.y * 6.2832),
    cos(uTime * aSeed.z * 0.83 + aSeed.y * 4.1)
  ) * 2.4 * settle;

  vec2 clip = (xy / uRes) * 2.0 - 1.0;
  gl_Position = vec4(clip.x, -clip.y, 0.0, 1.0);

  float z = mix(pf.z, pt.z, e);
  gl_PointSize = (1.5 + aSeed.w * 1.4) * uDpr * (1.0 + z * 0.45);

  float twinkle = 0.75 + 0.25 * sin(uTime * aSeed.z * 1.6 + aSeed.y * 6.2832);
  vColor = aColor;
  vAlpha = mix(uAlphaFrom, uAlphaTo, e) * twinkle * uAlphaMul;
}`

const FRAG = `#version 300 es
precision highp float;
in vec3 vColor;
in float vAlpha;
out vec4 frag;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.16, d) * vAlpha;
  if (a < 0.004) discard;
  frag = vec4(vColor, a);
}`

// Resolve a CSS color string (oklch tokens included) to linear-ish RGB floats
// by letting canvas 2D do the parsing.
function cssToRgb(css: string, scratch: CanvasRenderingContext2D): [number, number, number] {
  scratch.fillStyle = css
  scratch.fillRect(0, 0, 1, 1)
  const [r, g, b] = scratch.getImageData(0, 0, 1, 1).data
  return [r / 255, g / 255, b / 255]
}

// Welcome-page backdrop, after enchanted-twin's voice visualizer: one
// persistent swarm of ~20k GPU particles. It rests as faint full-screen dust,
// condenses into a construction — a spinning sphere or torus, a solid sun,
// bird, paper plane — holds, then either morphs straight into the next
// construction or relaxes back into dust. While `calm` (the user is typing),
// it stays dust and dims.
export function PixelField({
  calm = false,
  shapes,
}: {
  calm?: boolean
  shapes?: (keyof typeof SHAPES)[]
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const calmRef = useRef(calm)
  const reducedMotion = useReducedMotion()
  const playlist = (shapes?.length ? shapes : Object.keys(SHAPES)) as string[]
  const playlistKey = playlist.join(',')

  useEffect(() => {
    calmRef.current = calm
  }, [calm])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const gl = canvas.getContext('webgl2', { alpha: true, antialias: false })
    if (!gl) return

    /* ---- program ---- */
    const compile = (type: number, src: string) => {
      const sh = gl.createShader(type)
      if (!sh) return null
      gl.shaderSource(sh, src)
      gl.compileShader(sh)
      if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
        console.error('PixelField shader:', gl.getShaderInfoLog(sh))
        return null
      }
      return sh
    }
    const vs = compile(gl.VERTEX_SHADER, VERT)
    const fs = compile(gl.FRAGMENT_SHADER, FRAG)
    if (!vs || !fs) return
    const prog = gl.createProgram()
    gl.attachShader(prog, vs)
    gl.attachShader(prog, fs)
    gl.linkProgram(prog)
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
      console.error('PixelField link:', gl.getProgramInfoLog(prog))
      return
    }
    gl.useProgram(prog)
    gl.enable(gl.BLEND)
    gl.blendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
    const U = (name: string) => gl.getUniformLocation(prog, name)
    const uni = {
      res: U('uRes'),
      dpr: U('uDpr'),
      time: U('uTime'),
      progress: U('uProgress'),
      swirl: U('uSwirl'),
      alphaMul: U('uAlphaMul'),
      alphaFrom: U('uAlphaFrom'),
      alphaTo: U('uAlphaTo'),
      centerFrom: U('uCenterFrom'),
      centerTo: U('uCenterTo'),
      scaleFrom: U('uScaleFrom'),
      scaleTo: U('uScaleTo'),
      rotFrom: U('uRotFrom'),
      rotTo: U('uRotTo'),
    }

    /* ---- particle buffers ---- */
    const scratch = document.createElement('canvas')
    scratch.width = scratch.height = 1
    const sctx = scratch.getContext('2d', { willReadFrequently: true })
    if (!sctx) return
    const styles = getComputedStyle(document.documentElement)
    const palette = [
      ...Array.from({ length: 5 }, (_, i) =>
        cssToRgb(styles.getPropertyValue(`--color-rainbow-${i + 1}`).trim(), sctx),
      ),
      cssToRgb(styles.getPropertyValue('--color-primary').trim(), sctx),
    ]

    const fromArr = new Float32Array(N_MAX * 3)
    const toArr = new Float32Array(N_MAX * 3)
    const seedArr = new Float32Array(N_MAX * 4)
    const colorArr = new Float32Array(N_MAX * 3)
    for (let i = 0; i < N_MAX; i++) {
      seedArr[i * 4] = Math.random()
      seedArr[i * 4 + 1] = Math.random()
      seedArr[i * 4 + 2] = rand(0.3, 1.2)
      seedArr[i * 4 + 3] = Math.random()
      // mostly sage, the rainbow ramp as confetti, with lightness jitter
      const rgb = palette[Math.random() < 0.62 ? 5 : Math.floor(rand(0, 5))]
      const jitter = rand(0.82, 1.18)
      colorArr[i * 3] = Math.min(1, rgb[0] * jitter)
      colorArr[i * 3 + 1] = Math.min(1, rgb[1] * jitter)
      colorArr[i * 3 + 2] = Math.min(1, rgb[2] * jitter)
    }

    const vao = gl.createVertexArray()
    gl.bindVertexArray(vao)
    const attr = (name: string, size: number, data: Float32Array, usage: number) => {
      const buf = gl.createBuffer()
      gl.bindBuffer(gl.ARRAY_BUFFER, buf)
      gl.bufferData(gl.ARRAY_BUFFER, data, usage)
      const loc = gl.getAttribLocation(prog, name)
      gl.enableVertexAttribArray(loc)
      gl.vertexAttribPointer(loc, size, gl.FLOAT, false, 0, 0)
      return buf
    }
    const fromBuf = attr('aFrom', 3, fromArr, gl.DYNAMIC_DRAW)
    const toBuf = attr('aTo', 3, toArr, gl.DYNAMIC_DRAW)
    attr('aSeed', 4, seedArr, gl.STATIC_DRAW)
    attr('aColor', 3, colorArr, gl.STATIC_DRAW)

    /* ---- frames (the from/to coordinate spaces) ---- */
    const frameFrom = { cx: 0, cy: 0, scale: 1, rot: 0, rotSpeed: 0, wobble: false, alpha: 0.3 }
    const frameTo = { cx: 0, cy: 0, scale: 1, rot: 0, rotSpeed: 0, wobble: false, alpha: 0.3 }
    let progress = 1
    let morphDur = 1.8
    let swirl = 0
    let wobblePhase = 0

    let width = 0
    let height = 0
    let drawCount = N_MAX

    const ambient = (out: Float32Array) => {
      const scale = Math.max(width, height, 1) * 0.62
      for (let i = 0; i < N_MAX; i++) {
        out[i * 3] = (rand(0, width) - width / 2) / scale
        out[i * 3 + 1] = -(rand(0, height) - height / 2) / scale
        out[i * 3 + 2] = rand(-0.25, 0.25)
      }
      return scale
    }

    // 1 at the field's edges, 0 at the masked clearing's center
    const maskParam = (x: number, y: number) =>
      Math.hypot((x - width * MASK.cx) / MASK.rx, (y - height * MASK.cy) / MASK.ry)

    const clearOfMask = (cx: number, cy: number, r: number) => {
      if (maskParam(cx, cy) < 0.9) return false
      for (let i = 0; i < 8; i++) {
        const a = (i / 8) * TAU
        if (maskParam(cx + Math.cos(a) * r, cy + Math.sin(a) * r) < 0.66) return false
      }
      return true
    }

    // Largest construction radius (down to a readable floor) that keeps the
    // site inside the viewport and out of the masked clearing.
    const fitRadius = (cx: number, cy: number, want: number) => {
      for (const f of [1, 0.85, 0.7, 0.55]) {
        const r = want * f
        if (r < 100) break
        if (
          cx > r + 16 &&
          cx < width - r - 16 &&
          cy > r + 16 &&
          cy < height - r - 16 &&
          clearOfMask(cx, cy, r)
        )
          return r
      }
      return 0
    }

    // Commit the finished `to` state as the new `from`, then aim at the next.
    const beginMorph = (gen: (out: Float32Array) => void, target: typeof frameTo, dur: number, sw: number) => {
      fromArr.set(toArr)
      gl.bindBuffer(gl.ARRAY_BUFFER, fromBuf)
      gl.bufferSubData(gl.ARRAY_BUFFER, 0, fromArr)
      Object.assign(frameFrom, frameTo)
      gen(toArr)
      gl.bindBuffer(gl.ARRAY_BUFFER, toBuf)
      gl.bufferSubData(gl.ARRAY_BUFFER, 0, toArr)
      Object.assign(frameTo, target)
      progress = 0
      morphDur = dur
      swirl = sw
      wobblePhase = rand(0, TAU)
    }

    let lastShape = -1
    const toAmbient = () => {
      let scale = 1
      beginMorph(
        (out) => {
          scale = ambient(out)
        },
        { cx: width / 2, cy: height / 2, scale: 1, rot: 0, rotSpeed: 0, wobble: false, alpha: 0.32 },
        1.9,
        0.25,
      )
      frameTo.scale = scale
    }

    const toConstruction = () => {
      const idx = (lastShape + 1 + Math.floor(rand(0, playlist.length - 1))) % playlist.length
      const def = SHAPES[playlist[idx]]
      const want = Math.min(280, Math.max(130, Math.min(width, height) * 0.27)) * def.scaleMul
      let cx = 0
      let cy = 0
      let r = 0
      for (let attempt = 0; attempt < 24; attempt++) {
        const x = rand(0, width)
        const y = rand(0, height)
        const fit = fitRadius(x, y, want)
        if (fit > r) {
          cx = x
          cy = y
          r = fit
          if (r === want) break
        }
      }
      if (r === 0) return false
      lastShape = idx
      beginMorph(
        (out) => def.gen(out, N_MAX),
        {
          cx,
          cy,
          scale: r,
          rot: 0,
          rotSpeed: def.spin ? (Math.random() < 0.5 ? -1 : 1) * rand(0.18, 0.3) : 0,
          wobble: !def.spin,
          alpha: def.alpha,
        },
        1.8,
        0.5,
      )
      return true
    }

    /* ---- sizing ---- */
    const resize = () => {
      const rect = canvas.getBoundingClientRect()
      if (!rect.width || !rect.height || (rect.width === width && rect.height === height)) return
      width = rect.width
      height = rect.height
      const dpr = window.devicePixelRatio || 1
      canvas.width = Math.round(width * dpr)
      canvas.height = Math.round(height * dpr)
      gl.viewport(0, 0, canvas.width, canvas.height)
      gl.uniform2f(uni.res, width, height)
      gl.uniform1f(uni.dpr, dpr)
      drawCount = Math.min(N_MAX, Math.max(9000, Math.round((width * height) / 42)))
    }
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(canvas)

    // boot as settled dust
    ambient(toArr)
    fromArr.set(toArr)
    Object.assign(frameTo, {
      cx: width / 2,
      cy: height / 2,
      scale: Math.max(width, height, 1) * 0.62,
      rot: 0,
      rotSpeed: 0,
      wobble: false,
      alpha: 0.32,
    })
    Object.assign(frameFrom, frameTo)
    gl.bindBuffer(gl.ARRAY_BUFFER, fromBuf)
    gl.bufferSubData(gl.ARRAY_BUFFER, 0, fromArr)
    gl.bindBuffer(gl.ARRAY_BUFFER, toBuf)
    gl.bufferSubData(gl.ARRAY_BUFFER, 0, toArr)

    const drawFrame = (t: number, alphaMul: number) => {
      gl.clearColor(0, 0, 0, 0)
      gl.clear(gl.COLOR_BUFFER_BIT)
      gl.uniform1f(uni.time, t)
      gl.uniform1f(uni.progress, progress)
      gl.uniform1f(uni.swirl, swirl)
      gl.uniform1f(uni.alphaMul, alphaMul)
      gl.uniform1f(uni.alphaFrom, frameFrom.alpha)
      gl.uniform1f(uni.alphaTo, frameTo.alpha)
      gl.uniform2f(uni.centerFrom, frameFrom.cx, frameFrom.cy)
      gl.uniform2f(uni.centerTo, frameTo.cx, frameTo.cy)
      gl.uniform1f(uni.scaleFrom, frameFrom.scale)
      gl.uniform1f(uni.scaleTo, frameTo.scale)
      gl.uniform1f(uni.rotFrom, frameFrom.rot)
      gl.uniform1f(uni.rotTo, frameTo.rot)
      gl.drawArrays(gl.POINTS, 0, drawCount)
    }

    if (reducedMotion) {
      drawFrame(0, 1)
      return () => observer.disconnect()
    }

    /* ---- scheduler ---- */
    // states: 'ambient' (dust) -> 'forming' -> 'holding' -> back
    let state: 'ambient' | 'forming' | 'holding' = 'ambient'
    let stateUntil = performance.now() / 1000 + rand(2, 3)
    let chain = 0
    let alphaMul = 1

    let raf = 0
    let lastMs = performance.now()
    const frame = (ms: number) => {
      raf = requestAnimationFrame(frame)
      const dt = Math.min(0.05, (ms - lastMs) / 1000)
      lastMs = ms
      const t = ms / 1000
      const calmNow = calmRef.current

      alphaMul += ((calmNow ? 0.5 : 1) - alphaMul) * (1 - Math.exp(-dt * 2.5))
      progress = Math.min(1, progress + dt / morphDur)

      for (const f of [frameFrom, frameTo]) {
        if (f.wobble) f.rot = 0.3 * Math.sin(t * 0.4 + wobblePhase)
        else f.rot += f.rotSpeed * dt
      }

      if (state === 'ambient') {
        if (t > stateUntil && !calmNow) {
          if (toConstruction()) state = 'forming'
          else stateUntil = t + rand(2, 4)
        }
      } else if (state === 'forming') {
        if (progress >= 1) {
          state = 'holding'
          stateUntil = t + rand(4, 5.5)
        }
      } else if (state === 'holding') {
        if (calmNow) stateUntil = Math.min(stateUntil, t + 0.4)
        if (t > stateUntil) {
          const morphOn = !calmNow && chain < 2 && Math.random() < 0.55
          if (morphOn && toConstruction()) {
            chain++
            state = 'forming'
          } else {
            toAmbient()
            chain = 0
            state = 'ambient'
            stateUntil = t + morphDur + rand(3, 6)
          }
        }
      }

      drawFrame(t, alphaMul)
    }
    raf = requestAnimationFrame(frame)

    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
    }
  }, [reducedMotion, playlistKey])

  return (
    <canvas
      ref={canvasRef}
      aria-hidden
      // size-full is load-bearing: a canvas is a replaced element, so inset-0
      // alone leaves it at its intrinsic (attribute) size
      className="pointer-events-none absolute inset-0 size-full"
      style={{ WebkitMaskImage: MASK_CSS, maskImage: MASK_CSS }}
    />
  )
}
