const GATHER_SWEEP = 0.22
const RELEASE_SWEEP = 0.26
const FLIGHT = 0.5
export const PARTICLES_ZOOM_MS = 200
const SETTLED_MS = (RELEASE_SWEEP * 1.2 + FLIGHT + 0.07) * 1000
export const PARTICLES_ARRIVE_MS = SETTLED_MS
// How far the window steps back before it comes apart.
const STEP_BACK = 0.06
const GRAINS = 260_000

// Shared by both programs so a cell's picture and its grain agree on when it lifts off and lands.
const COMMON = `
precision highp float;
uniform vec2 size;
uniform float cell;
uniform float zoom;
uniform float leaving;
uniform float arriving;

float hash(vec2 p) {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
}

float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), u.x), mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x), u.y);
}

float gust(vec2 uv) {
  return noise(uv * vec2(4.0, 7.0)) - 0.5;
}

// How far cell \`index\`'s grain has been drawn into the ring, 0 in place and 1 in orbit.
// Cells lift off in a wave spreading from the top left, where the switcher is.
float gathered(vec2 index) {
  vec2 uv = (index + 0.5) * cell / size;
  float start = ${GATHER_SWEEP} * (length(uv) * 0.7 + gust(uv) * 0.35 + 0.17) + hash(index) * 0.07;
  return clamp((leaving - start) / ${FLIGHT}, 0.0, 1.0);
}

// How far the grain has come back out to its cell in the new window, 1 when landed.
// Cells are released left to right.
float released(vec2 index) {
  if (arriving < 0.0) return 0.0;
  vec2 uv = (index + 0.5) * cell / size;
  float start = ${RELEASE_SWEEP} * (uv.x * 0.85 + gust(uv) * 0.35 + 0.17) + hash(index) * 0.07;
  return clamp((arriving - start) / ${FLIGHT}, 0.0, 1.0);
}`

// The picture itself: each cell shows the old window until its grain lifts off,
// and the new one once its grain has landed.
const PICTURE_VERTEX = `
attribute vec2 corner;
void main() {
  gl_Position = vec4(corner, 0.0, 1.0);
}`

const PICTURE_FRAGMENT = `${COMMON}
uniform sampler2D from;
uniform sampler2D to;
uniform vec3 base;
uniform float density;

void main() {
  vec2 point = vec2(gl_FragCoord.x, size.y * density - gl_FragCoord.y) / density;
  point = (point - size * 0.5) / zoom + size * 0.5;
  vec2 index = floor(point / cell);
  vec3 color = base;
  if (point.x < 0.0 || point.y < 0.0 || point.x > size.x || point.y > size.y) {
  } else if (released(index) >= 1.0) {
    color = texture2D(to, point / size).rgb;
  } else if (gathered(index) <= 0.0) {
    color = texture2D(from, point / size).rgb;
  }
  gl_FragColor = vec4(color, 1.0);
}`

// The grains in the air. Every cell of the window is one grain: it is drawn into
// a tilted ring that turns around the middle of the window, changes from the old
// window's colour to the new one's while it orbits, and flies back out to the
// same cell.
const GRAIN_VERTEX = `${COMMON}
attribute vec2 index;
uniform sampler2D from;
uniform sampler2D to;
uniform vec3 base;
uniform vec3 accent;
uniform vec3 ink;
uniform float density;
uniform float clock;
varying vec4 tint;
varying float soft;

void main() {
  float back = released(index);
  float along = gathered(index) * (1.0 - back * back * (3.0 - 2.0 * back));
  if (along <= 0.0) {
    gl_Position = vec4(2.0, 2.0, 2.0, 1.0);
    return;
  }
  float pull = along * along * (3.0 - 2.0 * along);
  vec2 home = (index + 0.5) * cell;
  float a = hash(index + 7.3);
  float b = hash(index + 31.7);
  float c = hash(index + 53.1);

  float reach = min(size.x, size.y) * 0.4;
  float radius = reach * mix(0.42, 1.0, pow(b, 0.7));
  float angle = a * 6.2832 + clock * mix(3.4, 1.1, b);
  vec3 orbit = vec3(cos(angle) * radius, sin(angle) * radius, (c - 0.5) * reach * 0.16);
  float tilt = 1.08;
  orbit = vec3(orbit.x, orbit.y * cos(tilt) - orbit.z * sin(tilt), orbit.y * sin(tilt) + orbit.z * cos(tilt));
  float depth = (reach * 2.6) / (reach * 2.6 - orbit.z * pull);

  vec2 centre = size * 0.5;
  vec2 ring = centre + orbit.xy * depth;
  vec2 path = ring - home;
  vec2 point = home + path * pull + vec2(-path.y, path.x) * sin(pull * 3.14159) * (c - 0.5) * 0.35;
  point = (point - centre) * zoom + centre;

  vec3 color = mix(texture2D(from, home / size).rgb, texture2D(to, home / size).rgb, clamp(arriving / 0.3, 0.0, 1.0));
  float body = smoothstep(0.012, 0.06, distance(color, base));
  vec3 lit = mix(accent, ink, step(0.9, c) * 0.7);
  float arm = 0.5 + 0.5 * sin(a * 18.85 + b * 9.0);
  float shade = mix(1.0, (0.3 + 0.7 * arm * arm) * mix(0.75, 1.15, depth - 0.5), pull);
  tint = vec4(mix(color, lit, pull * mix(0.85, 0.2, body)), mix(mix(0.34, 0.85, pull), 1.0, body) * shade);
  soft = min(1.0, pull * 5.0);
  gl_PointSize = cell * density * zoom * mix(1.15, 1.5 * depth, pull);
  gl_Position = vec4(point.x / size.x * 2.0 - 1.0, 1.0 - point.y / size.y * 2.0, 0.0, 1.0);
}`

const GRAIN_FRAGMENT = `
precision highp float;
varying vec4 tint;
varying float soft;
void main() {
  float alpha = tint.a * mix(1.0, smoothstep(0.5, 0.2, length(gl_PointCoord - 0.5)), soft);
  gl_FragColor = vec4(tint.rgb * alpha, alpha);
}`

function ease(progress: number): number {
  const t = Math.min(1, Math.max(0, progress))
  return t * t * (3 - 2 * t)
}

function cssColor(variable: string): [number, number, number] {
  const context = document.createElement('canvas').getContext('2d', { willReadFrequently: true })!
  context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue(variable)
  context.fillRect(0, 0, 1, 1)
  const [red, green, blue] = context.getImageData(0, 0, 1, 1).data
  return [red / 255, green / 255, blue / 255]
}

function link(gl: WebGLRenderingContext, vertex: string, fragment: string): WebGLProgram {
  const program = gl.createProgram()
  for (const [type, source] of [[gl.VERTEX_SHADER, vertex], [gl.FRAGMENT_SHADER, fragment]] as const) {
    const shader = gl.createShader(type)!
    gl.shaderSource(shader, source)
    gl.compileShader(shader)
    gl.attachShader(program, shader)
  }
  gl.linkProgram(program)
  return program
}

// Steps `from`, a picture of the window, back a little on `canvas` and draws it,
// grain by grain, into a ring turning around the middle. `arrive` sends the
// grains back out as a picture of the new window and brings it forward, taking
// PARTICLES_ARRIVE_MS. Returns null where WebGL cannot do it.
export function runParticles(canvas: HTMLCanvasElement, from: HTMLCanvasElement) {
  const gl = canvas.getContext('webgl', { alpha: false, antialias: false })
  if (!gl || gl.getParameter(gl.MAX_VERTEX_TEXTURE_IMAGE_UNITS) < 2) return null
  const picture = link(gl, PICTURE_VERTEX, PICTURE_FRAGMENT)
  const grains = link(gl, GRAIN_VERTEX, GRAIN_FRAGMENT)
  if (!gl.getProgramParameter(picture, gl.LINK_STATUS) || !gl.getProgramParameter(grains, gl.LINK_STATUS)) return null

  const width = window.innerWidth
  const height = window.innerHeight
  const density = from.width / width
  const cell = Math.max(2.5, Math.sqrt((width * height) / GRAINS))
  canvas.width = from.width
  canvas.height = from.height
  gl.viewport(0, 0, canvas.width, canvas.height)
  gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)

  const columns = Math.ceil(width / cell)
  const rows = Math.ceil(height / cell)
  const indices = new Float32Array(columns * rows * 2)
  for (let i = 0; i < columns * rows; i++) {
    indices[i * 2] = i % columns
    indices[i * 2 + 1] = Math.floor(i / columns)
  }
  const corners = gl.createBuffer()
  gl.bindBuffer(gl.ARRAY_BUFFER, corners)
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
  const cells = gl.createBuffer()
  gl.bindBuffer(gl.ARRAY_BUFFER, cells)
  gl.bufferData(gl.ARRAY_BUFFER, indices, gl.STATIC_DRAW)

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

  const programs = [picture, grains].map((program) => {
    const at = (name: string) => gl.getUniformLocation(program, name)
    gl.useProgram(program)
    gl.uniform2f(at('size'), width, height)
    gl.uniform1f(at('cell'), cell)
    gl.uniform1f(at('density'), density)
    gl.uniform3f(at('base'), ...cssColor('--color-bg'))
    gl.uniform3f(at('accent'), ...cssColor('--color-primary'))
    gl.uniform3f(at('ink'), ...cssColor('--color-ink'))
    gl.uniform1i(at('from'), 0)
    gl.uniform1i(at('to'), 1)
    return { program, zoom: at('zoom'), leaving: at('leaving'), arriving: at('arriving'), clock: at('clock') }
  })
  const corner = gl.getAttribLocation(picture, 'corner')
  const index = gl.getAttribLocation(grains, 'index')

  const started = performance.now()
  let arrived = 0
  let frame = 0
  const draw = (now: number) => {
    const back = ease((now - started) / PARTICLES_ZOOM_MS)
    const forward = arrived ? ease((now - arrived - SETTLED_MS + PARTICLES_ZOOM_MS) / PARTICLES_ZOOM_MS) : 0
    for (const { program, zoom, leaving, arriving, clock } of programs) {
      gl.useProgram(program)
      gl.uniform1f(zoom, 1 - STEP_BACK * back * (1 - forward))
      gl.uniform1f(leaving, (now - started - PARTICLES_ZOOM_MS) / 1000)
      gl.uniform1f(arriving, arrived ? (now - arrived) / 1000 : -1)
      gl.uniform1f(clock, (now - started) / 1000)
    }
    gl.useProgram(picture)
    gl.disable(gl.BLEND)
    gl.bindBuffer(gl.ARRAY_BUFFER, corners)
    gl.enableVertexAttribArray(corner)
    gl.vertexAttribPointer(corner, 2, gl.FLOAT, false, 0, 0)
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)

    gl.useProgram(grains)
    gl.enable(gl.BLEND)
    gl.bindBuffer(gl.ARRAY_BUFFER, cells)
    gl.enableVertexAttribArray(index)
    gl.vertexAttribPointer(index, 2, gl.FLOAT, false, 0, 0)
    gl.drawArrays(gl.POINTS, 0, columns * rows)
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
