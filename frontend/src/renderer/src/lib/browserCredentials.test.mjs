import { expect, test } from 'bun:test'
import { build, resolveSync, spawn } from 'bun'
import { copyFile, mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { electron } from '../../../../scripts/test-electron.mjs'

const frontend = fileURLToPath(new URL('../../../../', import.meta.url))

test('real browser storage encrypts and restores credentials without gating approved access or resurrecting Forget', async () => {
  const runs = resolve(frontend, '../runs/browser-credentials')
  await mkdir(runs, { recursive: true })
  const output = await mkdtemp(join(runs, 'check-'))
  const result = await build({
    entrypoints: ['credential-fixture'],
    target: 'browser',
    define: { 'import.meta.env': '{"VITE_JAZ_API_URL":"origin"}' },
    plugins: [{
      name: 'credential-fixture',
      setup(builder) {
        builder.onResolve({ filter: /^credential-fixture$/ }, () => ({ path: 'entry', namespace: 'fixture' }))
        builder.onResolve({ filter: /^\//, namespace: 'fixture' }, (args) => ({ path: args.path, namespace: 'file' }))
        builder.onResolve({ filter: /^@\// }, ({ path }) => ({ path: resolveSync(join(frontend, 'src/renderer/src', path.slice(2)), frontend) }))
        builder.onResolve({ filter: /^@shared\// }, ({ path }) => ({ path: resolveSync(join(frontend, 'src/shared', path.slice(8)), frontend) }))
        builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
          contents: ['connection', 'api/client', 'browserCredentials'].map((file, index) =>
            `import * as module${index} from ${JSON.stringify(join(frontend, 'src/renderer/src/lib', file + '.ts'))}\nglobalThis.${['connection', 'api', 'credentials'][index]} = module${index}`).join('\n'),
          loader: 'ts',
        }))
      },
    }],
  })
  expect(result.success).toBe(true)
  await writeFile(join(output, 'renderer.js'), await result.outputs[0].text())
  await copyFile(join(frontend, 'scripts/browser-credentials-smoke.mjs'), join(output, 'main.mjs'))
  const child = spawn([electron, join(output, 'main.mjs')], {
    env: { ...process.env, JAZ_BROWSER_CREDENTIAL_TEST_DIR: output, TMPDIR: output },
    stdout: 'pipe',
    stderr: 'pipe',
  })
  const [exitCode, stdout, stderr] = await Promise.all([child.exited, new Response(child.stdout).text(), new Response(child.stderr).text()])
  await writeFile(join(output, 'electron.log'), stdout + stderr)
  expect({ exitCode, error: exitCode ? stdout + stderr : '' }).toEqual({ exitCode: 0, error: '' })
  expect(JSON.parse(await readFile(join(output, 'report.json'), 'utf8'))).toEqual({ visibleWindows: 0, focusedWindows: 0 })
}, 30_000)
