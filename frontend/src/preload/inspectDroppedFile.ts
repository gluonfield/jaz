import { ipcRenderer, webUtils } from 'electron'
import { INSPECT_DROPPED_FILE_CHANNEL, type InspectDroppedFile } from '@shared/fileActions'

export const inspectDroppedFile: InspectDroppedFile = async (file) => {
  try {
    const path = webUtils.getPathForFile(file)
    if (path) return await ipcRenderer.invoke(INSPECT_DROPPED_FILE_CHANNEL, path)
  } catch {
    return { kind: 'unavailable' }
  }
}
