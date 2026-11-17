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

/**
 * The brief as a table title: one line, first letter capital (Turkish rules: "i" becomes "İ"),
 * cut at a word and ended with "…" when it is longer than `max`. The full text goes in the tooltip.
 */
export function briefTitle(brief: string, max: number): string {
  const short = briefPreview(brief, max)
  if (short === '') return short
  return short.charAt(0).toLocaleUpperCase('tr-TR') + short.slice(1)
}

export type BadgeTone = 'neutral' | 'accent' | 'plan' | 'ok' | 'warn' | 'danger'

/** The badge of a table row: a short word and its colour. Unfinished campaigns are all just "Taslak". */
export function statusBadge(c: Pick<CampaignSummary, 'status' | 'segment_status'>): { text: string; tone: BadgeTone } {
  switch (c.status) {
    case 'completed':
      return { text: 'Tamamlandı', tone: 'ok' }
    case 'sending':
      return { text: 'Gönderiliyor', tone: 'accent' }
    case 'approved':
      return { text: 'Onaylandı', tone: 'accent' }
    case 'scheduled':
      return { text: 'Zamanlandı', tone: 'plan' }
    case 'cancelled':
      return { text: 'İptal edildi', tone: 'neutral' }
    case 'failed':
      return { text: 'Başlatılamadı', tone: 'danger' }
    case 'segment_ready':
    case 'content_ready':
      return { text: 'Taslak', tone: 'neutral' }
  }
  if (c.segment_status === 'unsupported') return { text: 'Desteklenmiyor', tone: 'warn' }
  if (c.segment_status === 'needs_clarification') return { text: 'Soru bekliyor', tone: 'warn' }
  return { text: 'Taslak', tone: 'neutral' }
}

export interface RowProgress {
  total: number
  sent: number
  failed: number
  /** Skipped (consent withdrawn) counts as handled but not as sent. */
  skipped: number
  pending: number
  handled: number
}

/** Both channels of a campaign added up, for the bar of its row (all zero before sending). */
export function rowProgress(d: DeliveryProgress | undefined): RowProgress {
  const t = deliveryTotals(d)
  return { total: t.total, sent: t.sent, failed: t.failed, skipped: t.skipped, pending: t.pending, handled: t.sent + t.failed + t.skipped }
}

/** The tooltip of a row's progress: the details the table has no column for. */
export function progressTitle(d: DeliveryProgress | undefined): string {
  const t = deliveryTotals(d)
  if (t.total === 0) return ''
  const parts = [`${t.sent} gönderildi`, `${t.failed} başarısız`, `${t.skipped} atlandı`, `${t.pending} bekliyor`]
  if (t.retries > 0) parts.push(`${t.retries} tekrar`, `${t.rateLimited} adet 429`)
  return parts.join(', ')
}

export interface LanguageCell {
  code: string
  /** "171", or "120/171" while some still wait; the audience size before sending. */
  text: string
}

/**
 * What the languages column shows: sent per language once sending has begun ("120/171" until it is
 * done), the audience per language before that. Languages nobody gets are left out.
 */
export function languageCells(
  c: Pick<CampaignSummary, 'delivery' | 'audience_languages'>,
  order: { code: string }[],
): LanguageCell[] {
  const sent = c.delivery?.languages
  if (sent) {
    return order
      .filter((l) => (sent[l.code]?.total ?? 0) > 0)
      .map((l) => {
        const p = sent[l.code]
        return { code: l.code.toUpperCase(), text: c.delivery!.done || p.sent === p.total ? String(p.sent) : `${p.sent}/${p.total}` }
      })
  }
  const audience = c.audience_languages
  if (!audience) return []
  return order.filter((l) => (audience[l.code] ?? 0) > 0).map((l) => ({ code: l.code.toUpperCase(), text: String(audience[l.code]) }))
}

/** A sentence that starts with a capital letter (Turkish rules: "i" becomes "İ"). Everything else stays as written. */
export function sentenceCase(text: string): string {
  const t = text.trim()
  return t === '' ? t : t.charAt(0).toLocaleUpperCase('tr-TR') + t.slice(1)
}
