const COVER_MS = 950
export const LIQUID_LEAVE_MS = 1000

const VERTEX = `
attribute vec2 corner;
void main() {
  gl_Position = vec4(corner, 0.0, 1.0);
}`

// A liquid that floods out from `origin` until it covers the window (`cover`),
// then drains from the centre outwards (`leave`). Both fronts are ragged by the
// same flow that moves the surface, and carry a bright crest.
const FRAGMENT = `
precision highp float;
uniform vec2 size;
uniform vec2 origin;
uniform float time;
uniform float cover;
uniform float leave;
uniform vec3 base;
uniform vec3 accent;

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

void main() {
  vec2 p = gl_FragCoord.xy;
  float diagonal = length(size);
  vec2 uv = p / max(size.x, size.y);
  vec2 q = uv * 3.2;
  vec2 warp = vec2(fbm(q + vec2(0.0, time * 0.42)), fbm(q + vec2(5.2, -time * 0.36)));
  float flow = fbm(q + 2.6 * warp + time * 0.18);
  float ragged = (fbm(uv * 7.0 + time * 0.5) - 0.5) * 0.3 + (warp.x - 0.5) * 0.22;

  float flood = cover * 1.75 - 0.3 - distance(p, origin) / diagonal - ragged;
  float drain = leave * 1.2 - 0.3 - distance(p, size * 0.5) / diagonal - ragged;
  float alpha = smoothstep(0.0, 0.035, flood) * (1.0 - smoothstep(0.0, 0.035, drain));
  float crest = (1.0 - smoothstep(0.0, 0.11, flood)) + smoothstep(-0.11, 0.0, drain);

  vec3 color = mix(base, accent, 0.1 + 0.55 * flow * flow);
  float caustic = pow(1.0 - abs(sin(flow * 9.0 + time * 1.4)), 4.0);
  color += mix(accent, vec3(1.0), 0.3) * caustic * 0.22;
  color = mix(color, mix(accent, vec3(1.0), 0.6), clamp(crest, 0.0, 1.0) * 0.85);
  gl_FragColor = vec4(color * alpha, alpha);
}`

function cssColor(variable: string): [number, number, number] {
  const context = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  if (!context) return [0, 0, 0]
  context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue(variable)
  context.fillRect(0, 0, 1, 1)
  const [red, green, blue] = context.getImageData(0, 0, 1, 1).data
  return [red / 255, green / 255, blue / 255]
}

function compile(gl: WebGLRenderingContext, type: number, source: string): WebGLShader {
  const shader = gl.createShader(type)!
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  return shader
}

// Starts the liquid on `canvas`, or returns null where WebGL is unavailable.
export function runLiquid(canvas: HTMLCanvasElement, origin: { x: number; y: number }) {
  const gl = canvas.getContext('webgl', { premultipliedAlpha: true })
  if (!gl) return null
  const program = gl.createProgram()
  gl.attachShader(program, compile(gl, gl.VERTEX_SHADER, VERTEX))
  gl.attachShader(program, compile(gl, gl.FRAGMENT_SHADER, FRAGMENT))
  gl.linkProgram(program)
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null
  gl.useProgram(program)
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer())
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
  const corner = gl.getAttribLocation(program, 'corner')
  gl.enableVertexAttribArray(corner)
  gl.vertexAttribPointer(corner, 2, gl.FLOAT, false, 0, 0)

  const scale = Math.min(window.devicePixelRatio, 1.5)
  canvas.width = Math.round(window.innerWidth * scale)
  canvas.height = Math.round(window.innerHeight * scale)
  gl.viewport(0, 0, canvas.width, canvas.height)
  const uniform = (name: string) => gl.getUniformLocation(program, name)
  gl.uniform2f(uniform('size'), canvas.width, canvas.height)
  gl.uniform2f(uniform('origin'), origin.x * scale, canvas.height - origin.y * scale)
  gl.uniform3f(uniform('base'), ...cssColor('--color-bg'))
  gl.uniform3f(uniform('accent'), ...cssColor('--color-primary'))
  const time = uniform('time')
  const cover = uniform('cover')
  const leave = uniform('leave')

  const started = performance.now()
  let leaving = 0
  let frame = 0
  const draw = (now: number) => {
    const covered = Math.min(1, (now - started) / COVER_MS)
    const left = leaving ? Math.min(1, (now - leaving) / LIQUID_LEAVE_MS) : 0
    gl.uniform1f(time, (now - started) / 1000)
    gl.uniform1f(cover, 1 - (1 - covered) ** 3)
    gl.uniform1f(leave, left * left * (3 - 2 * left))
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
    frame = requestAnimationFrame(draw)
  }
  frame = requestAnimationFrame(draw)

  return {
    leave: () => {
      leaving = performance.now()
    },
    stop: () => cancelAnimationFrame(frame),
  }
}
