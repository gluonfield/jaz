import { expect, test } from 'bun:test'
import * as Bun from 'bun'
import { access, copyFile, mkdir, mkdtemp, readFile, symlink, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { build } from 'vite'
import tailwindcss from '@tailwindcss/vite'

const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

test('hidden desktop restart paints progress, stays responsive and recovers through production IPC', async () => {
  const runs = resolve(frontend, '../runs/restart-update-feedback')
  await mkdir(runs, { recursive: true })
  const output = await mkdtemp(join(runs, 'ui-'))
  const mainBackend = join(frontend, 'src/main/backend.ts')
  const result = await Bun.build({
    entrypoints: [join(frontend, 'scripts/updater-smoke/main.ts'), join(frontend, 'src/preload/index.ts')],
    outdir: output,
    target: 'node',
    format: 'cjs',
    naming: '[name].[ext]',
    external: ['electron', 'electron-updater'],
    plugins: [{
      name: 'update-fixture-boundaries',
      setup(build) {
        build.onResolve({ filter: /^\.\/backend$/ }, (args) => args.importer === join(frontend, 'src/main/updater.ts')
          ? { path: join(frontend, 'scripts/updater-smoke/backend.ts') }
          : undefined)
        for (const [filter, file] of [[/^electron$/, 'backend-electron.ts'], [/^node:child_process$/, 'process.ts']]) {
          build.onResolve({ filter }, (args) => args.importer === mainBackend
            ? { path: join(frontend, 'scripts/updater-smoke', file) }
            : undefined)
        }
        build.onResolve({ filter: /^@(?:main|shared|preload)?\// }, (args) => {
          const file = args.path.replace('@main/', 'src/main/').replace('@shared/', 'src/shared/').replace('@preload/', 'src/preload/').replace('@/', 'src/renderer/src/')
          return { path: Bun.resolveSync(join(frontend, file), frontend) }
        })
      },
    }],
  })
  expect(result.success).toBe(true)
  await build({
    configFile: false,
    logLevel: 'silent',
    plugins: [tailwindcss()],
    resolve: { alias: { '@': join(frontend, 'src/renderer/src'), '@shared': join(frontend, 'src/shared') } },
    define: { 'import.meta.env': '{}', 'process.env.NODE_ENV': '"development"' },
    build: {
      outDir: output,
      emptyOutDir: false,
      lib: { entry: join(frontend, 'scripts/updater-smoke/renderer.tsx'), formats: ['es'], fileName: () => 'renderer.js', cssFileName: 'style' },
    },
  })
  await writeFile(join(output, 'index.html'), '<!doctype html><link rel="stylesheet" href="style.css"><div id="root"></div><script type="module" src="renderer.js"></script>')
  const electron = createRequire(import.meta.url)('electron')
  await copyFile(join(frontend, 'scripts/updater-smoke/bootstrap.mjs'), join(output, 'bootstrap.mjs'))
  await symlink(join(frontend, 'node_modules'), join(output, 'node_modules'), 'dir')
  await access(join(output, 'main.js'))
  await access(join(output, 'index.js'))
  const preflight = spawnSync(electron, ['-e', `require.resolve('electron-updater')`], { cwd: output, env: { ...process.env, ELECTRON_RUN_AS_NODE: '1' }, encoding: 'utf8' })
  expect({ status: preflight.status, error: preflight.stderr }).toEqual({ status: 0, error: '' })
  const observer = process.platform === 'darwin' ? Bun.spawn(['swift', '-module-cache-path', join(output, 'swift-cache'), join(frontend, 'scripts/updater-smoke/observe.swift'), output], {
    env: { ...process.env, TMPDIR: output },
    stdout: 'pipe',
    stderr: 'pipe',
  }) : undefined
  if (observer) {
    const deadline = performance.now() + 20_000
    while (!await Bun.file(join(output, 'os-ready')).exists()) {
      if (observer.exitCode !== null || performance.now() > deadline) {
        observer.kill()
        throw new Error(`OS observer failed: ${await new Response(observer.stderr).text()}`)
      }
      await Bun.sleep(20)
    }
  }
  const child = Bun.spawn([electron, join(output, 'bootstrap.mjs')], {
    cwd: frontend,
    env: { ...process.env, JAZ_UPDATER_SMOKE_DIR: output },
    stdout: 'pipe',
    stderr: 'pipe',
  })
  await writeFile(join(output, 'spawned-pid'), String(child.pid))
  const [exitCode, stdout, stderr] = await Promise.all([child.exited, new Response(child.stdout).text(), new Response(child.stderr).text()])
  await writeFile(join(output, 'electron.log'), `${stdout}\n${stderr}`)
  if (observer) {
    await writeFile(join(output, 'os-stop'), '')
    expect(await observer.exited).toBe(0)
    const os = JSON.parse(await readFile(join(output, 'os-report.json'), 'utf8'))
    expect(os.visibleWindows).toBe(0)
    expect(os.frontmostChanges).toBe(0)
  }
  expect({ exitCode, output: exitCode ? `${stdout}\n${stderr}` : '' }).toEqual({ exitCode: 0, output: '' })
  const report = JSON.parse(await readFile(join(output, 'report.json'), 'utf8'))
  expect(report.visibleWindows).toBe(0)
  expect(report.focusedWindows).toBe(0)
  expect(report.paintsDuringShutdown).toBeGreaterThan(0)
  expect(report.heartbeatsDuringShutdown).toBeGreaterThan(100)
  expect(report.preparationMs).toBeGreaterThanOrEqual(5_000)
  console.log(`Restart UI evidence: ${output}`)
}, 60_000)
