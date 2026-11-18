import type { Light, SystemHealth } from '../api/system'

export type HealthState = Light | 'checking'

export interface HealthItem {
  key: 'api' | 'worker' | 'db'
  label: string
  state: HealthState
}

export const healthWord = (s: HealthState) =>
  s === 'ok' ? 'çalışıyor' : s === 'down' ? 'yanıt vermiyor' : s === 'unknown' ? 'bilinmiyor' : 'kontrol ediliyor'

/** The services /api/system reports. When the api itself cannot be reached nothing is known about the others. */
export function healthItems(system: SystemHealth | null, unreachable: boolean): HealthItem[] {
  const rest = (v: Light | undefined): HealthState => (unreachable ? 'unknown' : (v ?? 'checking'))
  return [
    { key: 'api', label: 'API', state: unreachable ? 'down' : (system?.api ?? 'checking') },
    { key: 'worker', label: 'Worker', state: rest(system?.worker) },
    { key: 'db', label: 'Veritabanı', state: rest(system?.database) },
  ]
}

export const allHealthy = (items: HealthItem[]) => items.every((i) => i.state === 'ok')

/** The services that are down, or unknown because the api is down. */
export const downItems = (items: HealthItem[]) => items.filter((i) => i.state === 'down' || i.state === 'unknown')

/** The tooltip: one line per service, then the model. */
export function healthTitle(items: HealthItem[], llm: string | null): string {
  const lines = items.map((i) => `${i.label}: ${healthWord(i.state)}`)
  if (llm) lines.push(`LLM: ${llm}`)
  return lines.join('\n')
}
