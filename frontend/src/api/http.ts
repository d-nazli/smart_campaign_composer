import type { Report } from '../types'

/** An error response from the API: {"error": {"code", "message", "details"}}. */
export class ApiError extends Error {
  status: number
  code: string
  details?: { report?: Report; attempts?: string[][] }

  constructor(status: number, code: string, message: string, details?: ApiError['details']) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

/** Turns a non-2xx response into an ApiError (falling back for non-JSON bodies). */
export async function toApiError(res: Response): Promise<ApiError> {
  try {
    const body = (await res.json()) as { error?: { code?: string; message?: string; details?: ApiError['details'] } }
    if (body.error) {
      return new ApiError(res.status, body.error.code ?? 'error', body.error.message ?? res.statusText, body.error.details)
    }
  } catch {
    // not JSON
  }
  return new ApiError(res.status, 'http_error', `İstek başarısız oldu (${res.status})`)
}

export async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(url, {
      ...init,
      headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    })
  } catch {
    throw new ApiError(0, 'network', 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.')
  }
  if (!res.ok) {
    throw await toApiError(res)
  }
  if (res.status === 204) return undefined as T // no body (DELETE)
  return (await res.json()) as T
}
