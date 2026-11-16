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
    case 'scheduled':
      return 'Zamanlandı'
    case 'approved':
      return 'Onaylandı'
    case 'cancelled':
      return 'İptal edildi'
    case 'failed':
      return 'Başlatılamadı'
    case 'sending':
      return 'Gönderiliyor'
    case 'completed':
      return 'Tamamlandı'
  }
  if (c.segment_status === 'unsupported') return 'Desteklenmiyor'
  if (c.segment_status === 'needs_clarification') return 'Soru bekliyor'
  return 'Taslak'
}

/**
 * What the planned time of a campaign is called on its card (null when it was never scheduled).
 * A campaign that has started keeps its time: it says when it was meant to start.
 */
export function scheduleCaption(c: Pick<CampaignSummary, 'status' | 'scheduled_at'>): string | null {
  if (!c.scheduled_at) return null
  switch (c.status) {
    case 'scheduled':
      return 'Zamanlandı'
    case 'cancelled':
      return 'İptal edilen zaman'
    case 'failed':
      return 'Planlanan zaman'
    default:
      return 'Zamanlanan gönderim'
  }
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

export type Tone = 'draft' | 'scheduled' | 'sending' | 'done' | 'attention'

/** Colour group of a campaign's badge: draft, sending, done, or stuck on something the user must fix. */
export function statusTone(c: Pick<CampaignSummary, 'status' | 'segment_status'>): Tone {
  if (c.status === 'completed') return 'done'
  if (c.status === 'scheduled') return 'scheduled'
  if (c.status === 'failed') return 'attention'
  if (c.status === 'approved' || c.status === 'sending') return 'sending'
  if (c.status === 'draft' && (c.segment_status === 'unsupported' || c.segment_status === 'needs_clarification')) return 'attention'
  return 'draft'
}

/** Totals over every campaign, for the cards at the top of the dashboard. */
export function overallTotals(list: CampaignSummary[]): Totals {
  const t = deliveryTotals(undefined)
  for (const c of list) {
    const d = deliveryTotals(c.delivery)
    t.total += d.total
    t.sent += d.sent
    t.failed += d.failed
    t.skipped += d.skipped
    t.pending += d.pending
    t.retries += d.retries
    t.rateLimited += d.rateLimited
  }
  return t
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
