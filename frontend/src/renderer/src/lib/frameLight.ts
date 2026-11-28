// How long a frame takes to burst open to the window's rim.
export const OPEN_MS = 620

const VERTEX = `
attribute vec2 corner;
void main() {
  gl_Position = vec4(corner, 0.0, 1.0);
}`

// A frame of light drawn over the window: a rounded `rect` (centre and half
// extents, in CSS pixels from the window's centre) with up to three echoes
// trailing it, over a `veil` of `ground` that hides the page. With `open` set
// the frame's inside is clear, and `bloom` raises a nebula and sparks around it
// that `spread` carries outwards. `field` fills the veil with that nebula at
// rest, as a dawn the frame casts above `horizon` when its second part is set.
// Only the first `arc` of the frame, clockwise from top centre, is fully lit;
// the remainder glows at `rest`.
const FRAGMENT = `
precision highp float;
uniform vec2 size;
uniform float density;
uniform float time;
uniform vec4 rect;
uniform float radius;
uniform vec4 echoA;
uniform vec4 echoB;
uniform vec4 echoC;
uniform vec3 echoRadius;
uniform vec3 echoLum;
uniform float veil;
uniform vec3 ground;
uniform float open;
uniform float glow;
uniform float bloom;
uniform float spread;
uniform float dust;
uniform float field;
uniform vec2 horizon;
uniform float arc;
uniform float rest;
uniform float sweepRate;
uniform vec3 cool;
uniform vec3 ink;
uniform vec3 tint;
uniform float weight;

const vec3 WARM = vec3(1.0, 0.7, 0.48);
const vec3 WHITE = vec3(1.0, 0.96, 0.9);

float hash(vec2 p) {
  p = fract(p * vec2(0.1031, 0.1030));
  p += dot(p, p.yx + 33.33);
  return fract((p.x + p.y) * p.x);
}

float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * f * (f * (f * 6.0 - 15.0) + 10.0);
  return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), u.x), mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x), u.y);
}

float fbm(vec2 p) {
  float sum = 0.0;
  float gain = 0.5;
  for (int octave = 0; octave < 6; octave++) {
    sum += gain * noise(p);
    p = mat2(1.6, 1.2, -1.2, 1.6) * p + 11.7;
    gain *= 0.5;
  }
  return sum;
}

float box(vec2 p, vec4 r, float corner) {
  float c = min(corner, min(r.z, r.w));
  vec2 q = abs(p - r.xy) - r.zw + c;
  return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - c;
}

float echo(vec2 p, vec4 r, float corner) {
  float trail = box(p, r, corner);
  return exp(-trail * trail / 1.6);
}

float sparks(vec2 p, float pitch, float seed) {
  vec2 grid = p / pitch;
  vec2 cell = floor(grid);
  float kind = hash(cell + seed);
  vec2 home = vec2(hash(cell + seed + 3.1), hash(cell + seed + 7.7));
  vec2 drift = 0.12 * vec2(sin(time * (0.8 + kind) + kind * 40.0), cos(time * (0.6 + kind) + kind * 23.0));
  float radius = mix(0.5, 1.5, kind * kind);
  float away = length(fract(grid) - 0.2 - 0.6 * home - drift) * pitch;
  float twinkle = 0.55 + 0.45 * sin(time * 5.0 + kind * 60.0);
  return step(0.3, kind) * smoothstep(radius + 0.7, radius - 0.4, away) * twinkle;
}

void main() {
  vec2 point = vec2(gl_FragCoord.x, size.y * density - gl_FragCoord.y) / density;
  vec2 p = point - size * 0.5;
  float edge = box(p, rect, radius);
  float outside = max(edge, 0.0);
  vec2 rel = p - rect.xy;

  float alpha = veil * (1.0 - open * smoothstep(0.75, -0.75, edge));
  vec3 color = ground * alpha;

  vec2 q = p / 240.0;
  q += 0.7 * vec2(fbm(q * 1.3 + vec2(0.0, time * 0.2)), fbm(q * 1.3 + vec2(5.2, -time * 0.17))) - 0.35;
  float cloud = fbm(q * 2.1);
  float wisps = 1.0 - abs(2.0 * fbm(q * 4.3 + 3.0) - 1.0);
  float body = smoothstep(0.3, 0.8, cloud) * (0.3 + 0.7 * wisps * wisps);
  float smoke = body * exp(-outside / mix(14.0, 130.0, spread));
  color += mix(cool, WARM, smoothstep(0.15, 0.6, smoke)) * smoke * 1.3 * bloom * step(0.0, edge);

  float rise = horizon.x - p.y;
  float above = step(0.0, rise) * exp(-max(abs(rel.x) - rect.z, 0.0) / 60.0);
  float calm = 0.12 + 0.88 * smoothstep(50.0, 250.0, abs(p.y));
  vec3 sky = mix(mix(cool, WARM, smoothstep(0.55, 1.0, body)), mix(WARM, cool, smoothstep(10.0, 190.0, rise)), horizon.y);
  color += sky * body * mix(calm, above * exp(-rise / 170.0), horizon.y) * field * alpha * 1.25;

  float specks = sparks(point, 15.0, 71.0) + sparks(point, 23.0, 0.0) + sparks(point, 37.0, 19.0) + 0.6 * sparks(point, 61.0, 43.0);
  color += WHITE * specks * exp(-outside / mix(80.0, 300.0, spread)) * dust * step(0.0, edge);
  color += WHITE * specks * 0.3 * field * mix(calm, above * exp(-rise / 320.0), horizon.y) * alpha;

  float sweep = pow(0.5 + 0.5 * cos(atan(rel.y, rel.x) - time * sweepRate), 8.0) * smoothstep(0.0, 12.0, rect.w);
  float turn = fract(atan(rel.x / max(rect.z, 1.0), -rel.y / max(rect.w, 1.0)) / 6.2831853 + 1.0);
  float lit = max(mix(rest, 1.0, smoothstep(arc + 0.012, arc - 0.012, turn) * smoothstep(0.0, 0.012, turn)), step(0.999, arc));
  float line = exp(-edge * edge / weight);
  float halo = (0.5 * exp(-abs(edge) / 6.0) + 0.14 * exp(-abs(edge) / 34.0)) * (edge > 0.0 ? 1.0 : mix(0.45, 0.06, open));
  color += (ink * line * (0.75 + 0.5 * sweep) + tint * halo * (0.55 + 0.9 * sweep + 0.8 * bloom)) * glow * lit;
  color += WARM * (echoLum.x * echo(p, echoA, echoRadius.x) + echoLum.y * echo(p, echoB, echoRadius.y) + echoLum.z * echo(p, echoC, echoRadius.z)) * glow * (1.0 - open);
  gl_FragColor = vec4(color, alpha);
}`

const REST = {
  rect: [0, 0, 0, 0],
  radius: 0,
  echoA: [0, 0, 0, 0],
  echoB: [0, 0, 0, 0],
  echoC: [0, 0, 0, 0],
  echoRadius: [0, 0, 0],
  echoLum: [0, 0, 0],
  veil: 0,
  ground: [0, 0, 0],
  open: 0,
  glow: 0,
  bloom: 0,
  spread: 0,
  dust: 0,
  field: 0,
  horizon: [0, 0],
  arc: 1,
  rest: 1,
  sweepRate: 5.5,
  cool: [0.6, 0.48, 0.95],
  ink: [1, 0.96, 0.9],
  tint: [1, 0.7, 0.48],
  weight: 1.1,
}

export type FrameLight = typeof REST

export const clamp = (value: number) => Math.min(1, Math.max(0, value))
export const between = (now: number, from: number, span: number) => clamp((now - from) / span)
export const lerp = (a: number, b: number, k: number) => a + (b - a) * k
export const mix = (a: number[], b: number[], k: number) => a.map((value, index) => lerp(value, b[index], k))
export const ease = (k: number) => (k < 0.5 ? 4 * k ** 3 : 1 - (-2 * k + 2) ** 3 / 2)

// The window's rim as a frame, and its corner radius.
export const rim = () => [0, 0, window.innerWidth / 2 - 1.5, window.innerHeight / 2 - 1.5]
export const RIM_RADIUS = 10

// A frame bursting open, `out` of the way (0..1) to the rim.
export function bursting(out: number) {
  return {
    open: out > 0 ? 1 : 0,
    closed: (1 - out) ** 4,
    bloom: clamp(out / 0.12) * (1 - clamp((out - 0.35) / 0.6)),
    spread: 1 - (1 - clamp(out / 0.5)) ** 2,
    glow: 1 - clamp((out - 0.55) / 0.45),
  }
}

// Draws the frame of light on `canvas`, which lies over the whole window.
// Returns null where WebGL cannot do it.
export function createFrameLight(canvas: HTMLCanvasElement) {
  const gl = canvas.getContext('webgl', { antialias: false })
  if (!gl) return null
  const program = gl.createProgram()
  for (const [type, source] of [
    [gl.VERTEX_SHADER, VERTEX],
    [gl.FRAGMENT_SHADER, FRAGMENT],
  ] as const) {
    const shader = gl.createShader(type)!
    gl.shaderSource(shader, source)
    gl.compileShader(shader)
    gl.attachShader(program, shader)
  }
  gl.linkProgram(program)
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null
  gl.useProgram(program)
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer())
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
  const corner = gl.getAttribLocation(program, 'corner')
  gl.enableVertexAttribArray(corner)
  gl.vertexAttribPointer(corner, 2, gl.FLOAT, false, 0, 0)
  const at = (name: string) => gl.getUniformLocation(program, name)
  const size = at('size')
  const density = at('density')
  const time = at('time')
  const uniforms = Object.keys(REST).map((name) => [name as keyof FrameLight, at(name)] as const)

  const resize = () => {
    canvas.width = Math.round(window.innerWidth * window.devicePixelRatio)
    canvas.height = Math.round(window.innerHeight * window.devicePixelRatio)
    gl.viewport(0, 0, canvas.width, canvas.height)
    gl.uniform2f(size, window.innerWidth, window.innerHeight)
    gl.uniform1f(density, window.devicePixelRatio)
  }
  resize()

  return {
    resize,
    draw(seconds: number, values: Partial<FrameLight>) {
      gl.uniform1f(time, seconds)
      for (const [name, location] of uniforms) {
        const value = values[name] ?? REST[name]
        if (typeof value === 'number') gl.uniform1f(location, value)
        else if (value.length === 2) gl.uniform2fv(location, value)
        else if (value.length === 3) gl.uniform3fv(location, value)
        else gl.uniform4fv(location, value)
      }
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
    },
  }
}
