import { mkdtemp, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { build } from 'vite'
import { createRequire } from 'node:module'
import tailwindcss from '@tailwindcss/vite'

const output = await mkdtemp(join(tmpdir(), 'jaz-browser-smoke-'))
for (const entry of ['main.ts', 'preload.ts']) {
  const result = await Bun.build({
    entrypoints: [resolve('scripts/browser-smoke', entry)],
    outdir: output,
    target: 'node',
    format: 'cjs',
    external: ['electron'],
    plugins: [{
      name: 'source-paths',
      setup(build) {
        build.onResolve({ filter: /^@(?:main|shared)?\// }, (args) => {
          const file = args.path.replace('@main/', 'src/main/').replace('@shared/', 'src/shared/').replace('@/', 'src/renderer/src/')
          return { path: Bun.resolveSync(resolve(file), process.cwd()) }
        })
      },
    }],
    define: { 'import.meta.env': '{}' },
  })
  if (!result.success) {
    throw new Error(result.logs.join('\n'))
  }
}
await build({
  configFile: false,
  plugins: [tailwindcss()],
  resolve: { alias: { '@': resolve('src/renderer/src'), '@shared': resolve('src/shared') } },
  define: { 'import.meta.env': '{}', 'process.env.NODE_ENV': '"production"' },
  build: {
    outDir: output,
    emptyOutDir: false,
    lib: { entry: resolve('scripts/browser-smoke/renderer.tsx'), formats: ['es'], fileName: () => 'renderer.js', cssFileName: 'style' },
  },
})
await writeFile(join(output, 'index.html'), `<!doctype html><html><head><link rel="stylesheet" href="/style.css"><style>
body{margin:0}#root{height:100vh;display:flex;justify-content:flex-end}
</style></head><body><div id="root"></div><script type="module" src="/renderer.js"></script></body></html>`)
await writeFile(join(output, 'target.html'), `<!doctype html><html><head><title>Jaz browser control</title><style>
body{font:16px system-ui;background:#faf9f6;color:#242424;padding:42px;min-height:1600px}h1{font-size:36px;letter-spacing:-1px}button{margin:220px 0 0 260px;padding:16px 24px;border:0;border-radius:12px;background:#e6e3dd;font:inherit;white-space:nowrap}button:hover{background:#c5e7ca}output{display:block;margin-top:22px;color:#286c36}
</style></head><body><h1>Browser control</h1><p>The agent moves its cursor, then clicks the button.</p><button>Run this step</button><output>Ready</output><script>
window.clicks = 0
document.querySelector('button').addEventListener('click', (event) => {
  window.clicks += 1
  window.trusted = event.isTrusted
  document.querySelector('output').textContent = 'Step completed with a trusted browser click.'
})
</script></body></html>`)
const electron = process.env.JAZ_ELECTRON_BINARY || createRequire(import.meta.url)('electron') as string
console.log(`Browser smoke artifacts: ${output}`)
const processHandle = Bun.spawn(['go', 'test', '-tags=browserintegration', './internal/browsercontrol', '-run=TestDesktopElectron', '-count=1', '-v'], {
  cwd: resolve('../backend'),
  env: { ...process.env, JAZ_BROWSER_SMOKE_DIR: output, JAZ_ELECTRON_BINARY: electron },
  stdout: 'inherit',
  stderr: 'inherit',
})
process.exit(await processHandle.exited)
