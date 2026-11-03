// Persistent browser login: AES-GCM with a non-exportable key in IndexedDB.
// The server re-validates device status on every restore.
const DB_NAME = 'jaz.browserCredentials'
const STORE = 'credentials'
type Saved = { iv: Uint8Array; ciphertext: ArrayBuffer; deviceId: string }

async function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1)
    request.onupgradeneeded = () => request.result.createObjectStore(STORE)
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
    request.onblocked = () => reject(new Error('credential store blocked'))
  })
}

async function read<T>(id: string): Promise<T | undefined> {
  const db = await database()
  try {
    return await new Promise<T | undefined>((resolve, reject) => {
      const tx = db.transaction(STORE, 'readonly')
      const request = tx.objectStore(STORE).get(id)
      tx.oncomplete = () => resolve(request.result as T | undefined)
      tx.onabort = () => reject(tx.error)
      tx.onerror = () => reject(tx.error)
    })
  } finally { db.close() }
}

async function encryptionKey(): Promise<CryptoKey> {
  const existing = await read<CryptoKey>('key')
  if (existing) return existing
  const proposed = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
  const db = await database()
  try {
    return await new Promise<CryptoKey>((resolve, reject) => {
      // Atomic key selection when several tabs connect for the first time at once.
      const tx = db.transaction(STORE, 'readwrite')
      const store = tx.objectStore(STORE)
      const request = store.get('key')
      let selected: CryptoKey
      request.onsuccess = () => {
        selected = request.result || proposed
        if (!request.result) store.put(selected, 'key')
      }
      tx.oncomplete = () => resolve(selected)
      tx.onabort = () => reject(tx.error)
      tx.onerror = () => reject(tx.error)
    })
  } finally { db.close() }
}

export async function saveBrowserCredential(url: string, token: string, deviceId: string, current: () => boolean): Promise<void> {
  const key = await encryptionKey()
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: new TextEncoder().encode(url) }, key, new TextEncoder().encode(token))
  if (!current()) throw new Error('connection changed before save')
  const db = await database()
  try {
    await new Promise<void>((resolve, reject) => {
      if (!current()) { reject(new Error('connection changed before save')); return }
      const tx = db.transaction(STORE, 'readwrite')
      tx.objectStore(STORE).put({ iv, ciphertext, deviceId } satisfies Saved, url)
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error)
      tx.onerror = () => reject(tx.error)
    })
  } finally { db.close() }
}

export async function loadBrowserCredential(url: string): Promise<{ token: string; deviceId: string } | undefined> {
  const saved = await read<Saved>(url)
  if (!saved) return undefined
  const key = await read<CryptoKey>('key')
  if (!key) throw new Error('saved credential key missing')
  const plaintext = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: saved.iv as Uint8Array<ArrayBuffer>, additionalData: new TextEncoder().encode(url) }, key, saved.ciphertext)
  return { token: new TextDecoder().decode(plaintext), deviceId: saved.deviceId }
}

export async function forgetBrowserCredential(url: string): Promise<void> {
  const db = await database()
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite')
      tx.objectStore(STORE).delete(url)
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error)
      tx.onerror = () => reject(tx.error)
    })
  } finally { db.close() }
}
