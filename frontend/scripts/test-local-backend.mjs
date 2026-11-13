import { build, resolveSync } from 'bun'
import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath, pathToFileURL, URL } from 'node:url'
import { join, resolve } from 'node:path'
import process from 'node:process'
import console from 'node:console'
import { electron } from './test-electron.mjs'

const frontend = fileURLToPath(new URL('../', import.meta.url))

export async function checkLocalBackend() {
  const runs = resolve(frontend, '../runs/local-backend')
  await mkdir(runs, { recursive: true })
  const output = await mkdtemp(join(runs, 'check-'))
  const options = {
    outdir: output,
    plugins: [{
      name: 'source-paths',
      setup(builder) {
        builder.onResolve({ filter: /^@(?:main|shared)?\// }, ({ path }) => ({
          path: resolveSync(join(frontend, path.replace('@main/', 'src/main/').replace('@shared/', 'src/shared/').replace('@/', 'src/renderer/src/')), frontend),
        }))
      },
    }],
  }
  const bundles = await Promise.all([
    build({ ...options, entrypoints: [join(frontend, 'scripts/local-backend-smoke/main.ts')], naming: 'main.cjs', target: 'node', format: 'cjs', external: ['electron'] }),
    build({ ...options, entrypoints: [join(frontend, 'scripts/local-backend-smoke/preload.ts')], naming: 'preload.cjs', target: 'node', format: 'cjs', external: ['electron'] }),
    build({ ...options, entrypoints: [join(frontend, 'scripts/local-backend-smoke/renderer.tsx')], naming: 'renderer.js', target: 'browser', define: { 'import.meta.env': '{}', 'process.env.NODE_ENV': '"production"' } }),
  ])
  for (const bundle of bundles) {
    if (!bundle.success) throw new AggregateError(bundle.logs)
  }
  await writeFile(join(output, 'index.html'), '<!doctype html><script src="renderer.js" defer></script>')
  await writeFile(join(output, 'bootstrap.cjs'), `const { app, dialog } = require('electron')
if (process.platform === 'darwin') app.setActivationPolicy('prohibited')
dialog.showErrorBox = (title, content) => {
  console.error(title, content)
  app.exit(1)
}
try {
  require('./main.cjs')
} catch (error) {
  console.error(error)
  app.exit(1)
}
`)
  const env = { ...process.env, JAZ_LOCAL_BACKEND_TEST_DIR: output, TMPDIR: output }
  delete env.ELECTRON_RENDERER_URL
  const child = spawn(electron, [join(output, 'bootstrap.cjs')], { env, stdio: ['ignore', 'pipe', 'pipe'], timeout: 35_000, killSignal: 'SIGKILL' })
  let log = ''
  child.stdout.on('data', (chunk) => { log += chunk })
  child.stderr.on('data', (chunk) => { log += chunk })
  const code = await new Promise((resolve, reject) => {
    child.once('error', reject)
    child.once('exit', resolve)
  })
  await writeFile(join(output, 'electron.log'), log)
  if (code !== 0) throw new Error(`Local backend check failed (${code}): ${log}`)
  return JSON.parse(await readFile(join(output, 'report.json'), 'utf8'))
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  console.log(JSON.stringify(await checkLocalBackend()))
}
