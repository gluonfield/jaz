import { ipcRenderer, webUtils } from 'electron'
import { DROPPED_DIRECTORY_CHANNEL } from '@shared/fileActions'

export async function droppedDirectoryPath(file: File): Promise<string | null> {
  const path = webUtils.getPathForFile(file)
  return path ? ipcRenderer.invoke(DROPPED_DIRECTORY_CHANNEL, path) : null
}
