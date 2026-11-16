import type { Offer } from '../types'

// Mirrors backend/internal/campaign/schedule.go. The server stays the authority
// (it checks again when the campaign is approved); this lets the form say so
// while the person is still choosing.

export const MIN_LEAD_MS = 60 * 1000
export const MAX_AHEAD_MS = 30 * 24 * 60 * 60 * 1000

/** Turkey is UTC+3 all year. An offer's last day is a calendar date in that zone. */
const TRT_OFFSET_MS = 3 * 60 * 60 * 1000

/** The very end (last millisecond) of the offer's last day, or null when the offer has no last day. */
export function offerLastMoment(expiresAt: string | undefined): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(expiresAt ?? '')
  if (!m) return null
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])]
  // Midnight of the NEXT day in Istanbul, minus one millisecond.
  return new Date(Date.UTC(y, mo - 1, d + 1) - TRT_OFFSET_MS - 1)
}

const two = (n: number) => String(n).padStart(2, '0')

/** "2026-12-31T14:05" in the viewer's own zone: the value format of <input type="datetime-local">. */
export function toLocalInput(d: Date): string {
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}T${two(d.getHours())}:${two(d.getMinutes())}`
}

/** Reads a datetime-local value as the viewer's local time; null when empty or invalid. */
export function fromLocalInput(v: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/.test(v)) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d
}

/** "31.12.2026" from "2026-12-31". */
export function dayText(iso: string): string {
  const [y, m, d] = iso.split('-')
  return `${d}.${m}.${y}`
}

export interface ScheduleBounds {
  earliest: Date
  /** The earlier of: 30 days ahead, the offer's last moment. */
  latest: Date
  /** The limit comes from the offer, not from the 30 days. */
  limitedByOffer: boolean
}

export function scheduleBounds(offer: Offer | undefined, now: Date): ScheduleBounds {
  const earliest = new Date(now.getTime() + MIN_LEAD_MS)
  const thirty = new Date(now.getTime() + MAX_AHEAD_MS)
  const last = offerLastMoment(offer?.expires_at)
  if (last && last.getTime() < thirty.getTime()) return { earliest, latest: last, limitedByOffer: true }
  return { earliest, latest: thirty, limitedByOffer: false }
}

/** A sentence when the time cannot be used, otherwise null. Order: empty, too soon, after the offer, too far. */
export function checkSchedule(at: Date | null, offer: Offer | undefined, now: Date): string | null {
  if (!at) return 'Bir gönderim zamanı seçin.'
  if (at.getTime() < now.getTime() + MIN_LEAD_MS) return 'Gönderim zamanı en az 1 dakika sonrası olmalı.'
  const last = offerLastMoment(offer?.expires_at)
  if (last && at.getTime() > last.getTime()) {
    return `Gönderim zamanı teklifin son kullanma tarihinden (${dayText(offer!.expires_at!)}) sonra olamaz.`
  }
  if (at.getTime() > now.getTime() + MAX_AHEAD_MS) return 'Gönderim zamanı en fazla 30 gün sonrası olabilir.'
  return null
}

/** "16.11.2026 14:30" in the viewer's zone. */
export function formatWhen(iso: string | Date): string {
  const d = typeof iso === 'string' ? new Date(iso) : iso
  return d.toLocaleString('tr-TR', { dateStyle: 'short', timeStyle: 'short' })
}

/** "2 sa 5 dk sonra", "3 dk sonra", "birazdan"; for a time that has passed: "şimdi başlıyor". */
export function untilText(at: string | Date, now: Date): string {
  const ms = (typeof at === 'string' ? new Date(at) : at).getTime() - now.getTime()
  if (ms <= 0) return 'şimdi başlıyor'
  const minutes = Math.ceil(ms / 60000)
  if (minutes < 1) return 'birazdan'
  const days = Math.floor(minutes / 1440)
  const hours = Math.floor((minutes % 1440) / 60)
  const mins = minutes % 60
  if (days > 0) return `${days} gün ${hours} sa sonra`
  if (hours > 0) return mins > 0 ? `${hours} sa ${mins} dk sonra` : `${hours} sa sonra`
  return `${mins} dk sonra`
}

export interface QuickTime {
  label: string
  /** A datetime-local value in the viewer's zone. */
  value: string
}

const roundUpToFive = (d: Date): Date => {
  const r = new Date(d)
  r.setSeconds(0, 0)
  r.setMinutes(Math.ceil(r.getMinutes() / 5) * 5)
  return r
}

/** One-click times offered next to the picker: in an hour, this evening (18:00), tomorrow morning (09:00). */
export function quickTimes(now: Date): QuickTime[] {
  const inAnHour = roundUpToFive(new Date(now.getTime() + 60 * 60 * 1000))
  const evening = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 18, 0)
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 9, 0)
  const out: QuickTime[] = [{ label: '1 saat sonra', value: toLocalInput(inAnHour) }]
  // Only when it is still at least an hour away: "this evening" must not be a time that is almost now.
  if (evening.getTime() - now.getTime() >= 60 * 60 * 1000) out.push({ label: 'Bu akşam 18:00', value: toLocalInput(evening) })
  out.push({ label: 'Yarın 09:00', value: toLocalInput(tomorrow) })
  return out
}
