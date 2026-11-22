export class ApiError extends Error {
  status: number
  body?: unknown

  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.status = status
    this.body = body
  }
}

export async function readAPIResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    let parsedBody: unknown
    try {
      parsedBody = await res.json()
      const record = parsedBody as { error?: string } | null
      if (record?.error) {
        message = record.error
      }
    } catch {
      // non-JSON error body; keep the status text
    }
    throw new ApiError(res.status, message, parsedBody)
  }
  const body = await res.text()
  // 202 and 204 acknowledge a write; an empty 200 is still a broken contract.
  if (!body && (res.status === 202 || res.status === 204)) return undefined as T
  return JSON.parse(body) as T
}
