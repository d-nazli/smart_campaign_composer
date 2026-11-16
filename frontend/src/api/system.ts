export type Light = 'ok' | 'down' | 'unknown'

/** What /api/system reports: the api itself, its database, and the worker (asked by the api). */
export interface SystemHealth {
  api: Light
  database: Light
  worker: Light
}

export async function fetchSystem(): Promise<SystemHealth> {
  const res = await fetch('/api/system')
  if (!res.ok) throw new Error(`GET /api/system failed: ${res.status}`)
  return (await res.json()) as SystemHealth
}
