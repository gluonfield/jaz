import { contextBridge } from 'electron'
import { LOCAL_BACKEND_TRANSPORT_URL, LOCAL_BACKEND_URL } from '@shared/localBackend'

contextBridge.exposeInMainWorld('jaz', {
  apiBaseUrl: LOCAL_BACKEND_URL,
  localBackendTransport: LOCAL_BACKEND_TRANSPORT_URL,
  windowKind: 'main',
})
