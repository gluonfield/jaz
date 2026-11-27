const STORE = 'credentials'
type Saved = { iv: Uint8Array<ArrayBuffer>; ciphertext: ArrayBuffer }

async function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open('jaz.browserCredentials', 1)
    request.onupgradeneeded = () => request.result.createObjectStore(STORE)
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
    request.onblocked = () => reject(new Error('Browser credential storage is blocked'))
  })
}

async function transaction<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await database()
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(STORE, mode)
      const request = action(tx.objectStore(STORE))
      tx.oncomplete = () => resolve(request.result)
      tx.onabort = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}

function read<T>(id: string): Promise<T | undefined> {
  return transaction('readonly', (store) => store.get(id))
}

async function encryptionKey(): Promise<CryptoKey> {
  const existing = await read<CryptoKey>('key')
  if (existing) return existing
  const proposed = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
  const selected = await transaction<CryptoKey | undefined>('readwrite', (store) => {
    const request = store.get('key')
    request.onsuccess = () => {
      if (!request.result) store.put(proposed, 'key')
    }
    return request
  })
  return selected || proposed
}

export type StoredBrowserCredential = {
  token: string
  refreshToken?: string
}

function encodeCredentialPayload(token: string, refreshToken?: string): string {
  const refresh = refreshToken?.trim()
  if (!refresh) return token
  return JSON.stringify({ v: 2, token, refreshToken: refresh })
}

function decodeCredentialPayload(raw: string): StoredBrowserCredential {
  if (raw.startsWith('{"v":2,')) {
    try {
      const parsed = JSON.parse(raw) as { v?: unknown; token?: unknown; refreshToken?: unknown }
      if (parsed.v === 2 && typeof parsed.token === 'string') {
        const refreshToken = typeof parsed.refreshToken === 'string' && parsed.refreshToken.trim()
          ? parsed.refreshToken.trim()
          : undefined
        return refreshToken ? { token: parsed.token, refreshToken } : { token: parsed.token }
      }
    } catch {
      // Fall back to legacy plain token string.
    }
  }
  return { token: raw }
}

export async function saveBrowserCredential(
  url: string,
  token: string,
  current: () => boolean,
  refreshToken?: string,
): Promise<void> {
  const key = await encryptionKey()
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const payload = encodeCredentialPayload(token, refreshToken)
  const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: new TextEncoder().encode(url) }, key, new TextEncoder().encode(payload))
  await transaction('readwrite', (store) => {
    if (!current()) throw new Error('Connection changed before it could be saved')
    return store.put({ iv, ciphertext } satisfies Saved, url)
  })
}

export async function loadBrowserCredential(url: string): Promise<StoredBrowserCredential | undefined> {
  const saved = await read<Saved>(url)
  if (!saved) return undefined
  const key = await read<CryptoKey>('key')
  if (!key) throw new Error('Saved browser credential key is missing')
  const plaintext = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: saved.iv, additionalData: new TextEncoder().encode(url) }, key, saved.ciphertext)
  return decodeCredentialPayload(new TextDecoder().decode(plaintext))
}

export async function forgetBrowserCredential(url: string): Promise<void> {
  await transaction('readwrite', (store) => store.delete(url))
}
