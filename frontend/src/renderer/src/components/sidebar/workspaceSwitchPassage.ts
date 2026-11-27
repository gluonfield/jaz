const LIGHT_MS = 200
const SHRINK_AT_MS = 120
const SHRINK_MS = 480
export const PASSAGE_SWAP_MS = 380
export const PASSAGE_EXPAND_MS = 420
// The size the window shrinks to, as a share of itself.
const CARD = 0.62

const VERTEX = `
attribute vec2 corner;
void main() {
  gl_Position = vec4(corner, 0.0, 1.0);
}`

// The window becomes a card lit around its rim, floating in front of a drifting
// nebula and stars. `swap` passes a band of light across the card that turns
// the old window into the new one.
const FRAGMENT = `
precision highp float;
uniform sampler2D from;
uniform sampler2D to;
uniform vec2 size;
uniform vec3 base;
uniform vec3 accent;
uniform float density;
uniform float time;
uniform float shrink;
uniform float swap;
uniform float glow;

const vec3 WARM = vec3(1.0, 0.74, 0.56);
const vec3 SPACE = vec3(0.03, 0.03, 0.05);

float hash(vec2 p) {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
}

float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), u.x), mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x), u.y);
}

float fbm(vec2 p) {
  float sum = 0.0;
  float gain = 0.5;
  for (int octave = 0; octave < 5; octave++) {
    sum += gain * noise(p);
    p = p * 2.03 + 11.7;
    gain *= 0.5;
  }
  return sum;
}

float roundedBox(vec2 p, vec2 extent, float radius) {
  vec2 q = abs(p) - extent + radius;
  return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - radius;
}

float stars(vec2 p) {
  float lit = 0.0;
  for (int layer = 0; layer < 3; layer++) {
    float depth = float(layer);
    vec2 grid = p * (11.0 + 8.0 * depth) * (1.0 - 0.035 * time * (depth + 1.0)) + depth * 31.0;
    vec2 cell = floor(grid);
    float seed = hash(cell + depth * 17.0);
    vec2 star = (vec2(hash(cell + 1.3), hash(cell + 2.7)) - 0.5) * 0.7;
    float twinkle = 0.55 + 0.45 * sin(time * 3.0 + seed * 40.0);
    lit += step(0.7, seed) * smoothstep(0.1 * seed, 0.0, length(fract(grid) - 0.5 - star)) * twinkle;
  }
  return lit;
}

void main() {
  vec2 point = vec2(gl_FragCoord.x, size.y * density - gl_FragCoord.y) / density;
  vec2 centre = size * 0.5;
  float scale = mix(1.0, ${CARD}, shrink);
  float edge = roundedBox(point - centre, centre * scale, 28.0 * shrink);
  vec2 around = (point - centre) / size.y;
  float angle = atan(around.y, around.x);

  vec2 warp = vec2(fbm(around * 2.2 + vec2(0.0, time * 0.14)), fbm(around * 2.2 + vec2(4.0, -time * 0.11)));
  float smoke = fbm(around * 2.6 + 1.9 * warp + time * 0.06);
  float near = exp(-max(edge, 0.0) / (size.y * 0.17));
  vec3 nebula = mix(mix(vec3(0.56, 0.44, 0.95), accent, 0.35), WARM, smoothstep(0.32, 0.68, warp.y));
  float rays = pow(noise(vec2(angle * 9.0, time * 0.7)), 3.0);
  vec3 sky = mix(base, SPACE, smoothstep(0.0, 0.35, shrink));
  sky += nebula * smoothstep(0.22, 0.8, smoke) * near * 1.25 * shrink;
  sky += WARM * rays * near * 0.45 * shrink;
  sky += vec3(1.0, 0.94, 0.86) * stars(around) * shrink;

  vec2 uv = ((point - centre) / scale + centre) / size;
  float band = uv.x * 0.8 + uv.y * 0.2;
  float front = swap * 1.5 - 0.25;
  vec3 window = mix(texture2D(from, uv).rgb, texture2D(to, uv).rgb, smoothstep(0.0, 0.05, front - band));
  float flash = exp(-abs(band - front) * 16.0) * sin(swap * 3.14159);
  window = window * mix(1.0, 0.78, shrink) + WARM * flash * 0.85;

  vec3 color = mix(sky, window, smoothstep(0.75, -0.75, edge));
  float travel = pow(0.5 + 0.5 * cos(angle - time * 5.0), 6.0);
  color += WARM * (exp(-abs(edge) / 1.6) * (0.5 + 0.9 * travel) + exp(-abs(edge) / 16.0) * (0.16 + 0.5 * travel)) * glow;
  gl_FragColor = vec4(color, 1.0);
}`

function cssColor(variable: string): [number, number, number] {
  const context = document.createElement('canvas').getContext('2d', { willReadFrequently: true })!
  context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue(variable)
  context.fillRect(0, 0, 1, 1)
  const [red, green, blue] = context.getImageData(0, 0, 1, 1).data
  return [red / 255, green / 255, blue / 255]
}

const clamp = (value: number) => Math.min(1, Math.max(0, value))

// Lights the rim of `from`, a picture of the window, on `canvas` and shrinks it
// to a card in front of a nebula. `arrive` turns the card into a picture of the
// new window over PASSAGE_SWAP_MS and grows it back over PASSAGE_EXPAND_MS.
// Returns null where WebGL cannot do it.
export function runPassage(canvas: HTMLCanvasElement, from: HTMLCanvasElement) {
  const gl = canvas.getContext('webgl', { alpha: false, antialias: false })
  if (!gl) return null
  const program = gl.createProgram()
  for (const [type, source] of [[gl.VERTEX_SHADER, VERTEX], [gl.FRAGMENT_SHADER, FRAGMENT]] as const) {
    const shader = gl.createShader(type)!
    gl.shaderSource(shader, source)
    gl.compileShader(shader)
    gl.attachShader(program, shader)
  }
  gl.linkProgram(program)
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null
  gl.useProgram(program)

  canvas.width = from.width
  canvas.height = from.height
  gl.viewport(0, 0, canvas.width, canvas.height)
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer())
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
  const corner = gl.getAttribLocation(program, 'corner')
  gl.enableVertexAttribArray(corner)
  gl.vertexAttribPointer(corner, 2, gl.FLOAT, false, 0, 0)

  const upload = (unit: number, image: HTMLCanvasElement) => {
    gl.activeTexture(gl.TEXTURE0 + unit)
    gl.bindTexture(gl.TEXTURE_2D, gl.createTexture())
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, image)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  }
  upload(0, from)
  upload(1, from)

  const at = (name: string) => gl.getUniformLocation(program, name)
  gl.uniform2f(at('size'), window.innerWidth, window.innerHeight)
  gl.uniform1f(at('density'), from.width / window.innerWidth)
  gl.uniform3f(at('base'), ...cssColor('--color-bg'))
  gl.uniform3f(at('accent'), ...cssColor('--color-primary'))
  gl.uniform1i(at('from'), 0)
  gl.uniform1i(at('to'), 1)
  const uniforms = { time: at('time'), shrink: at('shrink'), swap: at('swap'), glow: at('glow') }

  const started = performance.now()
  let arrived = 0
  let frame = 0
  const draw = (now: number) => {
    const since = now - started
    const swapped = arrived ? clamp((now - arrived) / PASSAGE_SWAP_MS) : 0
    const grown = arrived ? clamp((now - arrived - PASSAGE_SWAP_MS) / PASSAGE_EXPAND_MS) : 0
    const shrunk = 1 - (1 - clamp((since - SHRINK_AT_MS) / SHRINK_MS)) ** 4
    const back = grown * grown * (3 - 2 * grown)
    gl.uniform1f(uniforms.time, since / 1000)
    gl.uniform1f(uniforms.shrink, shrunk * (1 - back))
    gl.uniform1f(uniforms.swap, swapped)
    gl.uniform1f(uniforms.glow, clamp(since / LIGHT_MS) * (1 - back))
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
    frame = requestAnimationFrame(draw)
  }
  draw(started)

  return {
    arrive: (to: HTMLCanvasElement) => {
      upload(1, to)
      arrived = performance.now()
    },
    stop: () => cancelAnimationFrame(frame),
  }
}
