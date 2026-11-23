import { contextBridge } from 'electron'
import { inspectDroppedFile } from '@preload/inspectDroppedFile'
import { LOCAL_BACKEND_TRANSPORT_URL, LOCAL_BACKEND_URL } from '@shared/localBackend'

const inspection = Promise.withResolvers<void>()
contextBridge.exposeInMainWorld('resumeDroppedFileInspection', inspection.resolve)
contextBridge.exposeInMainWorld('jaz', {
  apiBaseUrl: LOCAL_BACKEND_URL,
  localBackendTransport: LOCAL_BACKEND_TRANSPORT_URL,
  windowKind: 'main',
  inspectDroppedFile: async (file: File) => {
    await inspection.promise
    return inspectDroppedFile(file)
  },
})
