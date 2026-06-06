import { contextBridge } from 'electron'

const apiBaseUrl = process.env['JAZ_API_URL'] ?? 'http://localhost:8080'

contextBridge.exposeInMainWorld('jaz', { apiBaseUrl })
