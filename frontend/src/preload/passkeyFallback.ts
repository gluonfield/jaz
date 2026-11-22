import { contextBridge, ipcRenderer } from 'electron'

// Inlined rather than imported from @shared/preview so Electron's sandboxed frame
// preloads emit as self-contained single files without relative chunk requires.
const BROWSER_PASSKEY_FALLBACK_CHANNEL = 'jaz:browser-passkey-fallback'

// Stock macOS Electron lacks browser passkey entitlements and a WebAuthn modal UI:
// Chromium reports passkey/hybrid capabilities as available and hangs credentials.get/create
// waiting for an invisible USB security key. Report truthful capabilities and fail fast on
// non-roaming passkey ceremonies so sites fall back cleanly and Jaz can offer recovery actions.
if (window.isSecureContext) {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  const signal = `passkey:${Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')}`
  window.addEventListener(signal, () => {
    ipcRenderer.send(BROWSER_PASSKEY_FALLBACK_CHANNEL)
  }, true)
  contextBridge.executeInMainWorld({
    func: (notifySignal: string) => {
      const ROAMING = new Set(['usb', 'nfc', 'ble', 'smart-card'])
      const NativeEvent = Event
      const NativeDOMException = DOMException
      const NativePromise = Promise
      const dispatch = EventTarget.prototype.dispatchEvent.bind(window)
      const isRoamingOnly = (kind: 'get' | 'create', publicKey: Record<string, unknown>): boolean => {
        const hints = publicKey.hints
        if (Array.isArray(hints) && hints.includes('security-key')) {
          return true
        }
        if (kind === 'create') {
          const selection = publicKey.authenticatorSelection as { authenticatorAttachment?: unknown } | undefined
          return selection?.authenticatorAttachment === 'cross-platform'
        }
        const allow = publicKey.allowCredentials
        if (!Array.isArray(allow) || allow.length === 0) {
          return false
        }
        return allow.every((credential) => {
          const transports = (credential as { transports?: unknown } | null)?.transports
          return Array.isArray(transports) && transports.length > 0 &&
            transports.every((transport) => typeof transport === 'string' && ROAMING.has(transport))
        })
      }
      if (typeof PublicKeyCredential === 'function') {
        PublicKeyCredential.isConditionalMediationAvailable = () => NativePromise.resolve(false)
        const getCapabilities = PublicKeyCredential.getClientCapabilities
        if (typeof getCapabilities === 'function') {
          PublicKeyCredential.getClientCapabilities = async () => ({
            ...(await getCapabilities.call(PublicKeyCredential)),
            conditionalCreate: false,
            conditionalGet: false,
            hybridTransport: false,
            immediateGet: false,
            passkeyPlatformAuthenticator: false,
            userVerifyingPlatformAuthenticator: false,
          })
        }
      }
      if (typeof CredentialsContainer === 'function') {
        const NativeCredentialsContainer = CredentialsContainer
        const wrap = <K extends 'get' | 'create'>(kind: K) => {
          const native = NativeCredentialsContainer.prototype[kind] as (this: CredentialsContainer, options?: Record<string, unknown>) => Promise<Credential | null>
          if (typeof native !== 'function') {
            return
          }
          NativeCredentialsContainer.prototype[kind] = function (this: CredentialsContainer, options?: Record<string, unknown>) {
            if (!(this instanceof NativeCredentialsContainer) || !options || typeof options !== 'object' || !options.publicKey || typeof options.publicKey !== 'object') {
              return native.call(this, options)
            }
            const abort = options.signal as AbortSignal | undefined
            if (abort?.aborted) {
              return NativePromise.reject(abort.reason ?? new NativeDOMException('The operation was aborted.', 'AbortError'))
            }
            if (options.mediation === 'conditional') {
              return NativePromise.reject(new NativeDOMException('Conditional mediation is not supported.', 'NotSupportedError'))
            }
            if (options.mediation === 'immediate') {
              return NativePromise.reject(new NativeDOMException('No passkey is available in this browser.', 'NotAllowedError'))
            }
            if (isRoamingOnly(kind, options.publicKey as Record<string, unknown>)) {
              return native.call(this, options)
            }
            dispatch(new NativeEvent(notifySignal))
            return NativePromise.reject(new NativeDOMException('Platform passkeys are not available in this browser.', 'NotAllowedError'))
          } as typeof CredentialsContainer.prototype[K]
        }
        wrap('get')
        wrap('create')
      }
    },
    args: [signal],
  })
}
