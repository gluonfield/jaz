import { app } from 'electron'
import assert from 'node:assert/strict'
import console from 'node:console'
import process from 'node:process'

app.setActivationPolicy('prohibited')
app.setPath('userData', process.env.JAZ_BROWSER_PASSKEYS_CHECK_DIR)
app.whenReady().then(() => {
  try {
    assert.equal(process.versions.electron, '45.2.1', 'Expected the patched Electron 45.2.1 runtime')
    assert.equal(typeof app.enableWebAuthnBrowserAuthenticator, 'function', 'Runtime lacks the native browser passkey patch')
    assert.equal(typeof app.enableWebAuthnBrowserAuthenticator(), 'boolean')
    app.configureWebAuthn({ platformPasskeys: false })
    app.exit(0)
  } catch (error) {
    console.error(error.message)
    app.exit(1)
  }
})
