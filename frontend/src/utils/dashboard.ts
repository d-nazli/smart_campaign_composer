import type { CampaignSummary, DeliveryProgress } from '../types'

export type Page = 'compose' | 'dashboard'

/** Which page the address bar asks for. Anything unknown opens the composer. */
export function pageFromHash(hash: string): Page {
  return hash === '#/dashboard' ? 'dashboard' : 'compose'
}

/** A campaign's state in plain Turkish. A draft says WHY it is stuck. */
export function statusLabel(c: Pick<CampaignSummary, 'status' | 'segment_status'>): string {
  switch (c.status) {
    case 'segment_ready':
      return 'Kitle hazır'
    case 'content_ready':
      return 'Onay bekliyor'
    case 'approved':
      return 'Onaylandı'
    case 'sending':
      return 'Gönderiliyor'
    case 'completed':
      return 'Tamamlandı'
  }
  if (c.segment_status === 'unsupported') return 'Desteklenmiyor'
  if (c.segment_status === 'needs_clarification') return 'Soru bekliyor'
  return 'Taslak'
}

/** One line of the brief: whitespace collapsed, cut at a word with an ellipsis. */
export function briefPreview(brief: string, max: number): string {
  const text = brief.replace(/\s+/g, ' ').trim()
  if (text.length <= max) return text
  const cut = text.slice(0, max)
  const atWord = cut.lastIndexOf(' ')
  return (atWord > max / 2 ? cut.slice(0, atWord) : cut).replace(/[\s,.;:]+$/, '') + '…'
}

export interface Totals {
  total: number
  sent: number
  failed: number
  skipped: number
  pending: number
  retries: number
  rateLimited: number
}

/** Both channels added together (zeros before approval). */
export function deliveryTotals(d: DeliveryProgress | undefined): Totals {
  const t: Totals = { total: 0, sent: 0, failed: 0, skipped: 0, pending: 0, retries: 0, rateLimited: 0 }
  if (!d) return t
  for (const c of [d.sms, d.email]) {
    t.total += c.total
    t.sent += c.sent
    t.failed += c.failed
    t.skipped += c.skipped
    t.pending += c.pending
    t.retries += c.retries
    t.rateLimited += c.rate_limited
  }
  return t
}
