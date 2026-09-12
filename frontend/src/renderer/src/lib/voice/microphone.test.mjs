import { afterEach, expect, mock, test } from 'bun:test'
import { requestMicrophone } from './microphone'

const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')

afterEach(() => {
  for (const [name, descriptor] of [['window', originalWindow], ['navigator', originalNavigator]]) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
    } else {
      delete globalThis[name]
    }
  }
})

function installMicrophone(getUserMedia, desktop = true, platform = 'MacIntel') {
  Object.defineProperty(globalThis, 'window', { configurable: true, value: { jaz: desktop ? {} : undefined } })
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { platform, mediaDevices: { getUserMedia } } })
}

test('OS denial explains how to grant access and a later attempt can succeed', async () => {
  const denied = new DOMException('Permission denied', 'NotAllowedError')
  const stream = {}
  const getUserMedia = mock()
    .mockRejectedValueOnce(denied)
    .mockResolvedValueOnce(stream)
  installMicrophone(getUserMedia)
  const error = await requestMicrophone().catch((error) => error)
  expect(error.message).toBe(`Microphone access is blocked. Enable ${import.meta.env.DEV ? 'Electron' : 'Jaz'} in System Settings → Privacy & Security → Microphone, then restart Jaz.`)
  expect(error.cause).toBe(denied)
  expect(await requestMicrophone()).toBe(stream)
  expect(getUserMedia).toHaveBeenLastCalledWith({
    audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
  })
})

test('web and other desktops receive relevant permission instructions', async () => {
  const getUserMedia = mock().mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'))
  installMicrophone(getUserMedia, false)
  await expect(requestMicrophone()).rejects.toThrow('browser’s site permissions and system settings')
  installMicrophone(getUserMedia, true, 'Win32')
  await expect(requestMicrophone()).rejects.toThrow('Allow Jaz to use the microphone in your system privacy settings')
})

test('device failures retain their original diagnostic', async () => {
  const missing = new DOMException('Requested device not found', 'NotFoundError')
  installMicrophone(mock().mockRejectedValue(missing))
  expect(await requestMicrophone().catch((error) => error)).toBe(missing)
})
