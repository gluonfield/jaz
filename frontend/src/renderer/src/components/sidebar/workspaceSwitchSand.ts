const SWEEP = 0.4
const FLIGHT = 0.5
const ZOOM_MS = 200
const SETTLED_MS = (SWEEP * 1.2 + FLIGHT) * 1000
export const SAND_ARRIVE_MS = SETTLED_MS + ZOOM_MS
// How far the window steps back before it is blown away.
const STEP_BACK = 0.06
const GRAINS = 240_000

// Shared by both programs so a cell's picture and its grain agree on when it lifts off.
const COMMON = `
precision highp float;
uniform vec2 size;
uniform float cell;
uniform float zoom;

float hash(vec2 p) {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
}

float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), u.x), mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x), u.y);
}

// How far the grain of cell \`index\` is through its flight, \`clock\` seconds after the
// wind starts: 0 at rest, 1 gone. The wind crosses the window left to right in gusts.
float flight(vec2 index, float clock) {
  vec2 uv = (index + 0.5) * cell / size;
  float gust = noise(uv * vec2(4.0, 7.0)) - 0.5;
  float start = ${SWEEP} * (uv.x * 0.85 + gust * 0.35 + 0.17) + hash(index) * 0.07;
  return clamp((clock - start) / ${FLIGHT}, 0.0, 1.0);
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
uniform float leaving;
uniform float arriving;

void main() {
  vec2 point = vec2(gl_FragCoord.x, size.y * density - gl_FragCoord.y) / density;
  point = (point - size * 0.5) / zoom + size * 0.5;
  vec2 index = floor(point / cell);
  vec3 color = base;
  if (point.x < 0.0 || point.y < 0.0 || point.x > size.x || point.y > size.y) {
  } else if (arriving >= 0.0 && flight(index, arriving) >= 1.0) {
    color = texture2D(to, point / size).rgb;
  } else if (flight(index, leaving) <= 0.0) {
    color = texture2D(from, point / size).rgb;
  }
  gl_FragColor = vec4(color, 1.0);
}`

// The grains in the air. Leaving grains are carried off to the right; arriving
// ones come in from the left along the mirrored path and settle into place.
const GRAIN_VERTEX = `${COMMON}
attribute vec2 index;
uniform sampler2D image;
uniform vec3 base;
uniform vec3 accent;
uniform float density;
uniform float clock;
uniform float side;
varying vec4 tint;

void main() {
  float along = flight(index, clock);
  if (side < 0.0) along = 1.0 - along;
  if (along <= 0.0 || along >= 1.0) {
    gl_Position = vec4(2.0, 2.0, 2.0, 1.0);
    return;
  }
  vec2 home = (index + 0.5) * cell;
  float a = hash(index + 7.3);
  float b = hash(index + 31.7);
  float carried = along * along;
  vec2 drift = vec2(
    side * (0.3 + 0.7 * a) * size.x * 0.7 * carried,
    (b - 0.5) * size.y * 0.3 * carried - sin(along * 3.14159) * size.y * 0.07 * a
  );
  vec2 swirl = vec2(cos(b * 40.0 + along * 7.0), sin(a * 40.0 + along * 9.0)) * along * 34.0 * (0.4 + b);
  vec2 point = (home + drift + swirl - size * 0.5) * zoom + size * 0.5;

  vec3 color = texture2D(image, home / size).rgb;
  float body = smoothstep(0.012, 0.06, distance(color, base));
  float glint = step(0.82, b) * sin(along * 3.14159);
  tint = vec4(mix(color, mix(accent, vec3(1.0), 0.5), glint * 0.55), body * (1.0 - smoothstep(0.72, 1.0, along)));
  gl_PointSize = cell * density * zoom * mix(1.15, 0.75, along);
  gl_Position = vec4(point.x / size.x * 2.0 - 1.0, 1.0 - point.y / size.y * 2.0, 0.0, 1.0);
}`

const GRAIN_FRAGMENT = `
precision highp float;
varying vec4 tint;
void main() {
  gl_FragColor = vec4(tint.rgb * tint.a, tint.a);
}`

function cssColor(variable: string): [number, number, number] {
  const context = document.createElement('canvas').getContext('2d', { willReadFrequently: true })!
  context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue(variable)
  context.fillRect(0, 0, 1, 1)
  const [red, green, blue] = context.getImageData(0, 0, 1, 1).data
  return [red / 255, green / 255, blue / 255]
}

function ease(progress: number): number {
  const t = Math.min(1, Math.max(0, progress))
  return t * t * (3 - 2 * t)
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

// Steps `from`, a picture of the window, back a little on `canvas`, turns it to
// sand and blows it away.
// `arrive` blows a picture of the new window in, settles it and brings it forward, taking
// SAND_ARRIVE_MS. Returns null where WebGL cannot do it.
export function runSand(canvas: HTMLCanvasElement, from: HTMLCanvasElement) {
  const gl = canvas.getContext('webgl', { alpha: false, antialias: false })
  if (!gl || gl.getParameter(gl.MAX_VERTEX_TEXTURE_IMAGE_UNITS) < 1) return null
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

  const base = cssColor('--color-bg')
  const accent = cssColor('--color-primary')
  for (const program of [picture, grains]) {
    gl.useProgram(program)
    gl.uniform2f(gl.getUniformLocation(program, 'size'), width, height)
    gl.uniform1f(gl.getUniformLocation(program, 'cell'), cell)
    gl.uniform1f(gl.getUniformLocation(program, 'density'), density)
    gl.uniform3f(gl.getUniformLocation(program, 'base'), ...base)
  }
  gl.uniform3f(gl.getUniformLocation(grains, 'accent'), ...accent)
  gl.useProgram(picture)
  gl.uniform1i(gl.getUniformLocation(picture, 'from'), 0)
  gl.uniform1i(gl.getUniformLocation(picture, 'to'), 1)
  const corner = gl.getAttribLocation(picture, 'corner')
  const index = gl.getAttribLocation(grains, 'index')
  const zooms = [picture, grains].map((program) => gl.getUniformLocation(program, 'zoom'))
  const uniforms = {
    leaving: gl.getUniformLocation(picture, 'leaving'),
    arriving: gl.getUniformLocation(picture, 'arriving'),
    image: gl.getUniformLocation(grains, 'image'),
    clock: gl.getUniformLocation(grains, 'clock'),
    side: gl.getUniformLocation(grains, 'side'),
  }

  const started = performance.now()
  let arrived = 0
  let frame = 0
  const blow = (image: number, clock: number, side: number) => {
    gl.uniform1i(uniforms.image, image)
    gl.uniform1f(uniforms.clock, clock)
    gl.uniform1f(uniforms.side, side)
    gl.drawArrays(gl.POINTS, 0, columns * rows)
  }
  const draw = (now: number) => {
    const back = ease((now - started) / ZOOM_MS)
    const forward = arrived ? ease((now - arrived - SETTLED_MS) / ZOOM_MS) : 0
    const zoom = 1 - STEP_BACK * back * (1 - forward)
    const leaving = (now - started - ZOOM_MS) / 1000
    const arriving = arrived ? (now - arrived) / 1000 : -1
    gl.useProgram(picture)
    gl.uniform1f(zooms[0], zoom)
    gl.disable(gl.BLEND)
    gl.bindBuffer(gl.ARRAY_BUFFER, corners)
    gl.enableVertexAttribArray(corner)
    gl.vertexAttribPointer(corner, 2, gl.FLOAT, false, 0, 0)
    gl.uniform1f(uniforms.leaving, leaving)
    gl.uniform1f(uniforms.arriving, arriving)
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)

    gl.useProgram(grains)
    gl.uniform1f(zooms[1], zoom)
    gl.enable(gl.BLEND)
    gl.bindBuffer(gl.ARRAY_BUFFER, cells)
    gl.enableVertexAttribArray(index)
    gl.vertexAttribPointer(index, 2, gl.FLOAT, false, 0, 0)
    blow(0, leaving, 1)
    if (arrived) blow(1, arriving, -1)
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
