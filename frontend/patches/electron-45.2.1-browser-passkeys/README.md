# Native browser passkeys

This opt-in Electron 45.2.1 patch uses Chromium 156.0.8078.75's existing macOS browser authenticator. Chromium owns request validation, origins, credential discovery, extensions and native authentication. Electron's existing app-specific `platformPasskeys` mode remains separate.

`app.isWebAuthnBrowserAuthenticatorAvailable()` exposes Chromium's OS and signed-entitlement check. `app.configureWebAuthn({ browserPasskeys: true })` opts in, rejects unsupported processes and cannot be combined with `platformPasskeys`. Jaz enables it only when that native capability exists and returns true. Stock and unapproved builds retain their existing behavior.

## Build

Apply to an unmodified [Electron v45.2.1 source checkout](https://github.com/electron/electron/tree/v45.2.1). `base.sha256` pins every changed upstream file. From the Electron checkout, with `JAZ_SOURCE` pointing to this repository:

```sh
shasum -a 256 -c "$JAZ_SOURCE/frontend/patches/electron-45.2.1-browser-passkeys/base.sha256"
git apply --check "$JAZ_SOURCE/frontend/patches/electron-45.2.1-browser-passkeys/browser-passkeys.patch"
git apply "$JAZ_SOURCE/frontend/patches/electron-45.2.1-browser-passkeys/browser-passkeys.patch"
```

Build using Electron's [macOS instructions](https://www.electronjs.org/docs/latest/development/build-instructions-macos), producing an unpacked distribution containing `Electron.app`. The native patch must be compiled; changing Jaz's JavaScript cannot substitute for it.

After building Jaz's normal bundles, run the following from `frontend`:

```sh
JAZ_ELECTRON_DIST=/path/to/patched/dist JAZ_MAC_PROVISIONING_PROFILE=/path/to/approved.provisionprofile node_modules/.bin/electron-builder --config electron-builder.browser-passkeys.mjs --mac --arm64 --publish never
```

The opt-in configuration checks the runtime's native API before packaging and rejects stock Electron. Its signer copies the approved browser entitlement and matching application/team identifiers from the supplied profile onto the main app only. Existing helper entitlements and normal build configuration stay unchanged. Electron-builder embeds the profile and performs normal signing and notarization.

## Required acceptance

- [ ] Obtain Apple's managed [browser public-key credential entitlement](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.developer.web-browser.public-key-credential) and matching provisioning profile. An entitlement string alone does not grant access.
- [ ] Compile the patched Electron runtime and package an entitled build.
- [ ] Verify native availability, passkey consent/selection, cancellation and actual Google login in that signed build, including sign-in from a side-browser tab and popup.

The local checks cover profile selection, helper isolation, stock-runtime rejection, TypeScript and patch application. They do not establish native compilation or successful Google authentication. No account or credential changes are needed for those local checks.

Chromium's [existing implementation](https://github.com/chromium/chromium/blob/156.0.8078.75/device/fido/mac/icloud_keychain_sys.mm) includes Apple SPI used by Chromium; this patch reuses it rather than introducing a Jaz authentication bridge.
