export interface CanvasLocation {
  page: string
  navigation: string
}

export function readCanvasLocation(key: string): CanvasLocation {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key) ?? 'null')
    const record = value && typeof value === 'object' ? value as Record<string, unknown> : {}
    return {
      page: typeof record.page === 'string' ? record.page : '',
      navigation: typeof record.navigation === 'string' ? record.navigation : '',
    }
  } catch {
    return { page: '', navigation: '' }
  }
}

export function saveCanvasLocation(key: string, location: CanvasLocation): void {
  try {
    localStorage.setItem(key, JSON.stringify(location))
  } catch {
    return
  }
}
