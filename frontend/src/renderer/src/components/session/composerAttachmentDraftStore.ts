import type { Attachment } from '@/lib/api/types'
import type { ComposerDraftStorage } from '@/components/session/useComposerDraft'
import { uploadedAttachment, type ComposerAttachment } from '@/components/session/composerAttachmentTypes'

type StoredAttachment =
  | { kind: 'uploaded'; local_id: string; attachment: Attachment }
  | {
      kind: 'file'
      local_id: string
      name: string
      file: File
      mime_type?: string
      size?: number
      error?: string
    }

type DraftRecord = {
  key: string
  scope: ComposerDraftStorage
  updated_at: number
  items: StoredAttachment[]
}

const DB_NAME = 'jaz-composer-drafts'
const DB_VERSION = 2
const DRAFT_STORE = 'attachment_drafts'
const OLD_FILE_STORE = 'attachment_files'
const SESSION_NAMESPACE_KEY = 'jaz.composerDraftSession'
const SESSION_GC_AFTER_MS = 24 * 60 * 60 * 1000

let dbPromise: Promise<IDBDatabase> | null = null
let gcStarted = false
const pendingWrites = new Map<string, Promise<void>>()

export function loadAttachmentDraft(
  key: string | undefined,
  storage: ComposerDraftStorage,
): Promise<ComposerAttachment[]> {
  const recordKey = draftRecordKey(key, storage)
  if (!recordKey) return Promise.resolve([])
  return (async () => {
    try {
      await pendingWrites.get(recordKey)?.catch(() => {})
      const db = await openDB()
      const record = await request<DraftRecord | undefined>(
        db.transaction(DRAFT_STORE).objectStore(DRAFT_STORE).get(recordKey),
      )
      if (record) return attachmentsFromRecord(record)
    } catch {
      return readLegacyUploadedAttachments(key, storage)
    }
    const legacy = readLegacyUploadedAttachments(key, storage)
    if (legacy.length) void saveAttachmentDraft(key, storage, legacy).catch(() => {})
    return legacy
  })()
}

export function saveAttachmentDraft(
  key: string | undefined,
  storage: ComposerDraftStorage,
  items: ComposerAttachment[],
): Promise<void> {
  const recordKey = draftRecordKey(key, storage)
  if (!recordKey) return Promise.resolve()
  return queueWrite(recordKey, async () => {
    const storedItems = items.flatMap((item) => {
      const stored = storedAttachmentFrom(item)
      return stored ? [stored] : []
    })
    const db = await openDB()
    const store = db.transaction(DRAFT_STORE, 'readwrite').objectStore(DRAFT_STORE)
    if (storedItems.length === 0) {
      await request(store.delete(recordKey))
    } else {
      await request(store.put({ key: recordKey, scope: storage, updated_at: Date.now(), items: storedItems }))
    }
    clearLegacyUploadedAttachments(key, storage)
  })
}

export function deleteAttachmentDraft(key: string | undefined, storage: ComposerDraftStorage): Promise<void> {
  const recordKey = draftRecordKey(key, storage)
  if (!recordKey) return Promise.resolve()
  return queueWrite(recordKey, async () => {
    const db = await openDB()
    await request(db.transaction(DRAFT_STORE, 'readwrite').objectStore(DRAFT_STORE).delete(recordKey))
    clearLegacyUploadedAttachments(key, storage)
  })
}

function queueWrite(key: string, write: () => Promise<void>): Promise<void> {
  const next = (pendingWrites.get(key) ?? Promise.resolve()).catch(() => {}).then(write)
  const tracked = next.finally(() => {
    if (pendingWrites.get(key) === tracked) pendingWrites.delete(key)
  })
  pendingWrites.set(key, tracked)
  return tracked
}

function attachmentsFromRecord(record: DraftRecord): ComposerAttachment[] {
  if (!Array.isArray(record.items)) return []
  return record.items.flatMap<ComposerAttachment>((item) => {
    if (item.kind === 'uploaded') return [{ ...item.attachment, localId: item.local_id }]
    if (!item.file || !item.local_id || !item.name) return []
    return [{
      localId: item.local_id,
      name: item.name,
      file: item.file,
      ...(item.mime_type ? { mime_type: item.mime_type } : {}),
      ...(typeof item.size === 'number' && Number.isFinite(item.size) ? { size: item.size } : {}),
      ...(item.error ? { error: item.error } : {}),
    }]
  })
}

function storedAttachmentFrom(item: ComposerAttachment): StoredAttachment | null {
  const uploaded = uploadedAttachment(item)
  if (uploaded) return { kind: 'uploaded', local_id: item.localId, attachment: uploaded }
  if (!item.file) return null
  return {
    kind: 'file',
    local_id: item.localId,
    name: item.name,
    file: item.file,
    ...(item.mime_type ? { mime_type: item.mime_type } : {}),
    ...(typeof item.size === 'number' && Number.isFinite(item.size) ? { size: item.size } : {}),
    ...(item.error ? { error: item.error } : {}),
  }
}

function draftRecordKey(key: string | undefined, storage: ComposerDraftStorage): string {
  const legacyKey = legacyAttachmentKey(key)
  if (!legacyKey) return ''
  if (storage === 'local') return `local:${legacyKey}`
  return `session:${sessionNamespace()}:${legacyKey}`
}

function sessionNamespace(): string {
  try {
    let id = sessionStorage.getItem(SESSION_NAMESPACE_KEY)
    if (!id) {
      id = randomID()
      sessionStorage.setItem(SESSION_NAMESPACE_KEY, id)
    }
    return id
  } catch {
    return 'default'
  }
}

function randomID(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function legacyAttachmentKey(key: string | undefined): string {
  return key ? `${key}.attachments` : ''
}

function readLegacyUploadedAttachments(
  key: string | undefined,
  storage: ComposerDraftStorage,
): ComposerAttachment[] {
  const storedKey = legacyAttachmentKey(key)
  if (!storedKey) return []
  try {
    const parsed = JSON.parse(storageArea(storage).getItem(storedKey) ?? '[]') as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.flatMap((value) => {
      const attachment = attachmentFromUnknown(value)
      return attachment ? [{ ...attachment, localId: attachment.id }] : []
    })
  } catch {
    return []
  }
}

function clearLegacyUploadedAttachments(key: string | undefined, storage: ComposerDraftStorage): void {
  const storedKey = legacyAttachmentKey(key)
  if (!storedKey) return
  try {
    storageArea(storage).removeItem(storedKey)
  } catch {
    return
  }
}

function storageArea(kind: ComposerDraftStorage): Storage {
  return kind === 'local' ? localStorage : sessionStorage
}

function attachmentFromUnknown(value: unknown): Attachment | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  const id = typeof raw.id === 'string' ? raw.id : ''
  const name = typeof raw.name === 'string' ? raw.name : ''
  const uri = typeof raw.uri === 'string' ? raw.uri : ''
  if (!id || !name) return null
  return {
    id,
    name,
    ...(uri ? { uri } : {}),
    ...(typeof raw.mime_type === 'string' ? { mime_type: raw.mime_type } : {}),
    ...(typeof raw.size === 'number' && Number.isFinite(raw.size) ? { size: raw.size } : {}),
  }
}

async function openDB(): Promise<IDBDatabase> {
  if (typeof indexedDB === 'undefined') return Promise.reject(new Error('IndexedDB unavailable'))
  dbPromise ??= new Promise((resolve, reject) => {
    const open = indexedDB.open(DB_NAME, DB_VERSION)
    open.onupgradeneeded = () => {
      const db = open.result
      if (!db.objectStoreNames.contains(DRAFT_STORE)) {
        db.createObjectStore(DRAFT_STORE, { keyPath: 'key' })
      }
      if (db.objectStoreNames.contains(OLD_FILE_STORE)) {
        db.deleteObjectStore(OLD_FILE_STORE)
      }
    }
    open.onsuccess = () => {
      const db = open.result
      startSessionGC(db)
      resolve(db)
    }
    open.onerror = () => reject(open.error)
  })
  return dbPromise
}

function startSessionGC(db: IDBDatabase): void {
  if (gcStarted) return
  gcStarted = true
  void pruneOldSessionDrafts(db).catch(() => {})
}

function pruneOldSessionDrafts(db: IDBDatabase): Promise<void> {
  return new Promise((resolve, reject) => {
    const currentPrefix = `session:${sessionNamespace()}:`
    const cutoff = Date.now() - SESSION_GC_AFTER_MS
    const tx = db.transaction(DRAFT_STORE, 'readwrite')
    const store = tx.objectStore(DRAFT_STORE)
    const cursor = store.openCursor()
    cursor.onsuccess = () => {
      const current = cursor.result
      if (!current) return
      const record = current.value as Partial<DraftRecord>
      if (
        record.scope === 'session' &&
        typeof record.key === 'string' &&
        !record.key.startsWith(currentPrefix) &&
        typeof record.updated_at === 'number' &&
        record.updated_at < cutoff
      ) {
        current.delete()
      }
      current.continue()
    }
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
    tx.onabort = () => reject(tx.error)
  })
}

function request<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}
