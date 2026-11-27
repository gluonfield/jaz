const LIGHT_MS = 140
const GATHER_AT_MS = 100
const GATHER_MS = 440
export const PASSAGE_OPEN_MS = 560
// How far behind the frame each of its three echoes runs.
const ECHO_MS = 55

const VERTEX = `
attribute vec2 corner;
void main() {
  gl_Position = vec4(corner, 0.0, 1.0);
}`

// A frame of light drawn over the window. `gather` takes it from the window's
// rim (0) down to the name tile (1), trailing echoes, while the page dims
// beneath. With `open` set the frame's inside shows the new window, and
// `bloom` raises a nebula and sparks around it as it grows back.
const FRAGMENT = `
precision highp float;
uniform sampler2D from;
uniform sampler2D to;
uniform vec2 size;
uniform vec2 tile;
uniform float density;
uniform float time;
uniform float gather;
uniform vec3 echoes;
uniform float open;
uniform float glow;
uniform float bloom;

const vec3 WARM = vec3(1.0, 0.7, 0.48);
const vec3 WHITE = vec3(1.0, 0.96, 0.9);
const vec3 VIOLET = vec3(0.6, 0.48, 0.95);

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

float frame(vec2 p, float gathered) {
  vec2 extent = mix(size * 0.5 - 1.5, tile * 0.5, gathered);
  float radius = mix(10.0, 16.0, gathered);
  vec2 q = abs(p) - extent + radius;
  return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - radius;
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
  return step(0.45, kind) * smoothstep(radius + 0.7, radius - 0.4, away) * twinkle;
}

void main() {
  vec2 point = vec2(gl_FragCoord.x, size.y * density - gl_FragCoord.y) / density;
  vec2 p = point - size * 0.5;
  vec2 uv = point / size;
  float edge = frame(p, gather);
  float outside = max(edge, 0.0);
  float angle = atan(p.y, p.x);

  vec3 color = texture2D(from, uv).rgb * mix(1.0, 0.2, glow);
  color = mix(color, texture2D(to, uv).rgb, open * smoothstep(0.75, -0.75, edge));

  float lit = max(bloom, 0.3 * glow * gather);
  vec2 q = p / 240.0;
  q += 0.7 * vec2(fbm(q * 1.3 + vec2(0.0, time * 0.2)), fbm(q * 1.3 + vec2(5.2, -time * 0.17))) - 0.35;
  float cloud = fbm(q * 2.1);
  float wisps = 1.0 - abs(2.0 * fbm(q * 4.3 + 3.0) - 1.0);
  float near = exp(-outside / 120.0);
  float smoke = smoothstep(0.3, 0.8, cloud) * (0.3 + 0.7 * wisps * wisps) * near;
  color += mix(VIOLET, WARM, smoothstep(0.15, 0.6, smoke)) * smoke * 1.3 * bloom * step(0.0, edge);
  float dust = sparks(point, 23.0, 0.0) + sparks(point, 37.0, 19.0) + 0.6 * sparks(point, 61.0, 43.0);
  color += WHITE * dust * exp(-outside / 210.0) * lit * step(0.0, edge);

  float sweep = pow(0.5 + 0.5 * cos(angle - time * 5.5), 8.0);
  float line = exp(-edge * edge / 1.1);
  float halo = (0.5 * exp(-abs(edge) / 6.0) + 0.14 * exp(-abs(edge) / 34.0)) * (edge > 0.0 ? 1.0 : mix(0.45, 0.06, open));
  color += (WHITE * line * (0.75 + 0.5 * sweep) + WARM * halo * (0.55 + 0.9 * sweep + 0.8 * bloom)) * glow;
  for (int index = 0; index < 3; index++) {
    float behind = index == 0 ? echoes.x : index == 1 ? echoes.y : echoes.z;
    float trail = frame(p, behind);
    color += WARM * exp(-trail * trail / 1.6) * clamp(abs(behind - gather) * 7.0, 0.0, 1.0) * (0.42 - 0.11 * float(index)) * glow * (1.0 - open);
  }
  color += (hash(gl_FragCoord.xy) - 0.5) / 255.0;
  gl_FragColor = vec4(color, 1.0);
}`

const clamp = (value: number) => Math.min(1, Math.max(0, value))
const ease = (value: number) => (value < 0.5 ? 4 * value ** 3 : 1 - (-2 * value + 2) ** 3 / 2)

// Draws a frame of light round `from`, a picture of the window, on `canvas`
// and gathers it onto a tile of `tile` CSS pixels at the centre. `arrive` opens
// the frame back out over PASSAGE_OPEN_MS onto a picture of the new window.
// Returns null where WebGL cannot do it.
export function runPassage(canvas: HTMLCanvasElement, from: HTMLCanvasElement, tile: [number, number]) {
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
  gl.uniform2f(at('tile'), ...tile)
  gl.uniform1f(at('density'), from.width / window.innerWidth)
  gl.uniform1i(at('from'), 0)
  gl.uniform1i(at('to'), 1)
  const uniforms = { time: at('time'), gather: at('gather'), echoes: at('echoes'), open: at('open'), glow: at('glow'), bloom: at('bloom') }

  const started = performance.now()
  let arrived = 0
  let frame = 0
  const opened = (now: number) => (arrived ? clamp((now - arrived) / PASSAGE_OPEN_MS) : 0)
  const gathered = (now: number) => ease(clamp((now - started - GATHER_AT_MS) / GATHER_MS)) * (1 - ease(opened(now)))
  const draw = (now: number) => {
    const out = opened(now)
    gl.uniform1f(uniforms.time, (now - started) / 1000)
    gl.uniform1f(uniforms.gather, gathered(now))
    gl.uniform3f(uniforms.echoes, gathered(now - ECHO_MS), gathered(now - 2 * ECHO_MS), gathered(now - 3 * ECHO_MS))
    gl.uniform1f(uniforms.open, arrived ? 1 : 0)
    gl.uniform1f(uniforms.glow, clamp((now - started) / LIGHT_MS) * (1 - clamp((out - 0.8) / 0.2)))
    gl.uniform1f(uniforms.bloom, clamp(out / 0.2) * (1 - clamp((out - 0.5) / 0.45)))
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
