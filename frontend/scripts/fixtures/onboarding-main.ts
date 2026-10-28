import { app, BrowserWindow, systemPreferences } from 'electron'
import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { execFileSync } from 'node:child_process'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { loadComputerDriver } from '@main/computerDriver'
import { installComputerControl } from '@main/computerControl'
import { installSystemPermissions } from '@main/systemPermissions'

app.setActivationPolicy('prohibited')
app.on('window-all-closed', () => {})
const output = process.env.JAZ_PERMISSION_EVIDENCE!
app.setPath('userData', join(output, `profile-${process.pid}`))
const broken = process.env.JAZ_PERMISSION_BROKEN === '1'
const deadline = setTimeout(() => app.exit(1), 60000)
const html = join(app.getAppPath(), 'out/permissions/renderer/index.html')
app.whenReady().then(async () => {
  try {
    assert.equal(app.isPackaged, true)
    installComputerControl()
    // No runtime lease is admitted; the probe closes its own native driver explicitly.
    app.removeAllListeners('before-quit')
    installSystemPermissions()
    const native = new BrowserWindow({ show: false, webPreferences: { preload: join(app.getAppPath(), 'out/preload/index.js'), sandbox: false } })
    await native.loadFile(html, { search: 'native' })
    assert.equal(native.isVisible(), false)
    const permissions = await native.webContents.executeJavaScript('window.jaz.systemPermissions.status()')
    assert.equal(permissions.accessibility, systemPreferences.isTrustedAccessibilityClient(false) ? 'granted' : 'needed')
    assert.equal(permissions.screenRecording, systemPreferences.getMediaAccessStatus('screen') === 'granted' ? 'granted' : 'needed')
    assert.ok(['granted', 'needed', 'off'].includes(permissions.microphone))
    const computer = await native.webContents.executeJavaScript('window.jaz.computer.status()')
    let tools = 0
    let libraries: string[] = []
    let archivedImportFailure: string | undefined
    if (broken) {
      assert.equal(computer.available, false)
      assert.match(computer.reason, /Update or reinstall Jaz/)
      assert.doesNotMatch(computer.reason, /dlopen|app\.asar|libcua/)
      assert.match(await readFile(join(app.getPath('userData'), 'logs/main.log'), 'utf8'), /libcua_driver_sdk\.dylib/)
      native.destroy()
    } else {
      assert.ok(computer.driverVersion)
      assert.ok(computer.permissions)
      const sdk = await loadComputerDriver()
      const driver = sdk.CuaDriver.create(undefined)
      try {
        tools = JSON.parse(await driver.listToolsJson()).tools.length
        assert.ok(tools > 0)
        libraries = execFileSync('/usr/bin/vmmap', [String(process.pid)], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024, stdio: ['ignore', 'pipe', 'ignore'] }).split('\n').filter(line => /libcua_driver_sdk|cua_driver_node_runtime\.node/.test(line))
        assert.ok(libraries.some(line => line.includes('app.asar.unpacked') && line.includes('libcua_driver_sdk')))
        assert.ok(libraries.some(line => line.includes('app.asar.unpacked') && line.includes('cua_driver_node_runtime.node')))
      } finally {
        await driver.shutdown()
        driver.uniffiDestroy()
      }
      const archived = pathToFileURL(join(process.resourcesPath, 'app.asar/node_modules/@trycua/cua-driver/dist/index.js')).href
      await assert.rejects(import(archived), (error: Error) => {
        archivedImportFailure = error.message
        return /library open failed|dlopen/.test(error.message)
      })
      native.destroy()
      const window = new BrowserWindow({ show: false, width: 900, height: 900, webPreferences: { backgroundThrottling: false } })
      window.webContents.on('console-message', ({ message }) => console.log(message))
      await window.loadFile(html)
      window.webContents.debugger.attach('1.3')
      await window.webContents.debugger.sendCommand('Emulation.setFocusEmulationEnabled', { enabled: true })
      await window.webContents.executeJavaScript(`new Promise(resolve => {
        const ready = setInterval(() => {
          if (window.runPermissionStates) {
            clearInterval(ready)
            resolve()
          }
        }, 20)
      })`)
      const result = await window.webContents.executeJavaScript('window.runPermissionStates()')
      assert.equal(result.passed, true)
      for (const mode of ['computer', 'voice', 'granted', 'error', 'driver-error']) {
        await window.webContents.executeJavaScript(`window.renderPermissionState(${JSON.stringify(mode)})`)
        await writeFile(join(output, `${mode}.png`), (await window.webContents.capturePage()).toPNG())
      }
      assert.equal(window.isVisible(), false)
      window.destroy()
    }
    await writeFile(join(output, broken ? 'missing-library.json' : 'native.json'), JSON.stringify({ packaged: app.isPackaged, resources: process.resourcesPath, permissions, computer, tools, libraries, archivedImportFailure }, null, 2))
    console.log(JSON.stringify({ passed: true, broken, tools }))
    clearTimeout(deadline)
    app.quit()
  } catch (error) {
    console.error(error)
    app.exit(1)
  }
})
