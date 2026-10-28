import { mkdir, rename, rm, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { build } from 'vite'
import tailwindcss from '@tailwindcss/vite'

const output = resolve('../runs/onboarding-permissions')
const fixtures = resolve('out/permissions')
try {
  await mkdir(join(fixtures, 'renderer'), { recursive: true })
  await mkdir(output, { recursive: true })
  const aliases = { '@main': resolve('src/main'), '@shared': resolve('src/shared'), '@': resolve('src/renderer/src') }
  const probe = await Bun.build({
    entrypoints: ['scripts/fixtures/onboarding-main.ts'],
    target: 'node',
    format: 'cjs',
    external: ['electron', '@trycua/cua-driver'],
    alias: aliases,
    plugins: [{
      name: 'asset-paths',
      setup(builder) {
        builder.onResolve({ filter: /\?asset$/ }, args => ({ path: resolve(args.resolveDir, args.path.slice(0, -6)), namespace: 'asset' }))
        builder.onLoad({ filter: /.*/, namespace: 'asset' }, args => ({ contents: `export default ${JSON.stringify(args.path)}`, loader: 'js' }))
      },
    }],
  })
  if (!probe.success) throw new Error(probe.logs.join('\n'))
  await writeFile(join(fixtures, 'main.cjs'), await probe.outputs[0].text())
  await build({
    configFile: false,
    define: { 'process.env.NODE_ENV': '"production"', 'import.meta.env': '{}' },
    plugins: [tailwindcss()],
    resolve: { alias: aliases },
    build: {
      outDir: join(fixtures, 'renderer'),
      emptyOutDir: false,
      lib: { entry: resolve('scripts/fixtures/onboarding-renderer.tsx'), formats: ['es'], fileName: () => 'fixture.js', cssFileName: 'fixture' },
    },
  })
  await writeFile(join(fixtures, 'renderer/index.html'), `<!doctype html><html><head><link rel="stylesheet" href="fixture.css"></head><body><div id="root"></div><script type="module">
  if (!location.search) {
    window.fixtureState = { calls: [], current: {}, mode: 'ready' }
    window.jaz = {
      apiBaseUrl: 'http://127.0.0.1:1',
      systemPermissions: {
        status: async () => {
          const fixture = window.fixtureState
          if (fixture.mode === 'loading') await new Promise(resolve => fixture.resume = resolve)
          if (fixture.mode === 'error') throw new Error('dlopen /private/app.asar/library')
          return { ...fixture.current }
        },
        allow: async permission => {
          const fixture = window.fixtureState
          fixture.calls.push(permission)
          await new Promise(resolve => fixture.resume = resolve)
          if (fixture.mode === 'action-error') throw new Error('dlopen /private/app.asar/library')
          fixture.current[permission] = 'granted'
        },
      },
      computer: { status: async () => ({ available: false, reason: 'Computer tools couldn’t start. Update or reinstall Jaz, then try again.' }) },
    }
    await import('./fixture.js')
  }
  </script></body></html>`)
  await writeFile(join(output, 'package.log'), '')
  const packed = Bun.spawn([
    'bun', 'x', '--no-install', 'electron-builder', '--dir', '--mac', '--arm64',
    `-c.directories.output=${join(output, 'package')}`,
    '-c.appId=dev.wins.jaz.permission-verification',
    '-c.extraMetadata.main=out/permissions/main.cjs',
    '-c.mac.identity=null',
  ], { stdout: Bun.file(join(output, 'package.log')), stderr: 'inherit', env: { ...process.env, CSC_IDENTITY_AUTO_DISCOVERY: 'false' } })
  if (await packed.exited !== 0) throw new Error('Packaging failed')
  const bundle = join(output, 'package/mac-arm64/Jaz.app/Contents')
  for (const broken of [false, true]) {
    const dylib = join(bundle, 'Resources/app.asar.unpacked/node_modules/@trycua/cua-driver-darwin-arm64/libcua_driver_sdk.dylib')
    if (broken) await rename(dylib, dylib + '.held')
    try {
      const log = join(output, broken ? 'broken.log' : 'packaged.log')
      await writeFile(log, '')
      const check = Bun.spawn([join(bundle, 'MacOS/Jaz')], {
        env: { ...process.env, JAZ_PERMISSION_EVIDENCE: output, JAZ_PERMISSION_BROKEN: broken ? '1' : '' },
        stdout: Bun.file(log),
        stderr: 'inherit',
      })
      const deadline = setTimeout(() => check.kill('SIGKILL'), 60000)
      const exitCode = await check.exited
      clearTimeout(deadline)
      if (exitCode !== 0) throw new Error(`Packaged check failed (missing library: ${broken})`)
    } finally {
      if (broken) await rename(dylib + '.held', dylib)
    }
  }
  console.log(`Packaged onboarding evidence: ${output}`)
} finally {
  await rm(fixtures, { recursive: true, force: true })
}
