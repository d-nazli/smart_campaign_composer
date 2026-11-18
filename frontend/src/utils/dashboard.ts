import type { CampaignSummary, ChannelProgress, DeliveryProgress } from '../types'

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

export interface LocaleLine {
  /** The two largest locales and how many more: "tr-TR 828 · en-US 174 · +10". */
  text: string
  /** The same in pieces ("tr-TR 828", "en-US 174", "+10"), so a narrow column wraps between them. */
  parts: string[]
  /** Every locale, one per line, largest first (the tooltip). */
  title: string
}

/**
 * What the locales column shows: sent per locale once sending has begun ("120/171" until it is
 * done), the audience per locale before that. The two largest locales are named, the rest
 * counted ("+10"); the tooltip lists them all. Ties keep the table order. Codes are shown as
 * written (tr-TR): never upper-cased, which a Turkish page would turn "it" into "İT".
 */
export function localeLine(c: Pick<CampaignSummary, 'delivery' | 'audience_locales'>, order: { code: string }[]): LocaleLine | null {
  const rank = (code: string) => {
    const i = order.findIndex((l) => l.code === code)
    return i < 0 ? order.length : i
  }
  let rows: { code: string; size: number; text: string }[]
  const sent = c.delivery?.locales
  if (sent) {
    rows = Object.entries(sent)
      .filter(([, p]) => p.total > 0)
      .map(([code, p]) => ({ code, size: p.total, text: c.delivery!.done || p.sent === p.total ? String(p.sent) : `${p.sent}/${p.total}` }))
  } else {
    rows = Object.entries(c.audience_locales ?? {})
      .filter(([, n]) => n > 0)
      .map(([code, n]) => ({ code, size: n, text: String(n) }))
  }
  if (rows.length === 0) return null
  rows.sort((a, b) => b.size - a.size || rank(a.code) - rank(b.code) || a.code.localeCompare(b.code))
  const shown = rows.slice(0, 2).map((r) => `${r.code} ${r.text}`)
  if (rows.length > 2) shown.push(`+${rows.length - 2}`)
  return { text: shown.join(' · '), parts: shown, title: rows.map((r) => `${r.code} ${r.text}`).join('\n') }
}

/** A sentence that starts with a capital letter (Turkish rules: "i" becomes "İ"). Everything else stays as written. */
export function sentenceCase(text: string): string {
  const t = text.trim()
  return t === '' ? t : t.charAt(0).toLocaleUpperCase('tr-TR') + t.slice(1)
}

/** A campaign that is being sent right now: approved or sending, with something still waiting. */
export function isSending(c: Pick<CampaignSummary, 'status' | 'delivery'>): boolean {
  return (c.status === 'approved' || c.status === 'sending') && !c.delivery?.done
}

/** "26 sn" / "1 dk 5 sn". */
export function duration(seconds: number): string {
  const s = Math.max(1, Math.round(seconds))
  return s < 60 ? `${s} sn` : s % 60 === 0 ? `${s / 60} dk` : `${Math.floor(s / 60)} dk ${s % 60} sn`
}

/**
 * One channel's average speed so far: accepted messages over the time between the first and the
 * last one, and that time in seconds. null before anything was accepted.
 */
export function channelAverage(ch: ChannelProgress): { perSecond: number; seconds: number } | null {
  if (!ch.first_sent_at || !ch.last_sent_at || ch.sent === 0) return null
  const seconds = (Date.parse(ch.last_sent_at) - Date.parse(ch.first_sent_at)) / 1000
  return { perSecond: ch.sent / Math.max(1, seconds), seconds }
}

export interface ChannelRates {
  sms: number
  email: number
}

/**
 * The speed of a campaign, per channel (the limit is 100/s for each channel, not for both together):
 * while it is sending, the live rate of each channel ("SMS 98/sn · E-posta 97/sn");
 * once it is done, the average and how long it took ("ort. 95/sn, 26 sn'de bitti").
 * null when there is nothing to say (never sent, or no live rate measured yet).
 */
export function sendSpeed(c: Pick<CampaignSummary, 'status' | 'delivery'>, live?: ChannelRates): string | null {
  const d = c.delivery
  if (!d) return null
  if (isSending(c)) {
    if (!live) return null
    const parts: string[] = []
    if (d.sms.total > 0) parts.push(`SMS ${Math.round(live.sms)}/sn`)
    if (d.email.total > 0) parts.push(`E-posta ${Math.round(live.email)}/sn`)
    return parts.length ? parts.join(' · ') : null
  }
  if (c.status !== 'completed') return null
  let first = Infinity
  let last = -Infinity
  const averages: number[] = []
  for (const ch of [d.sms, d.email]) {
    const a = channelAverage(ch)
    if (!a) continue
    first = Math.min(first, Date.parse(ch.first_sent_at!))
    last = Math.max(last, Date.parse(ch.last_sent_at!))
    averages.push(a.perSecond)
  }
  if (averages.length === 0) return null
  const avg = averages.reduce((a, b) => a + b, 0) / averages.length
  return `ort. ${Math.round(avg)}/sn, ${duration((last - first) / 1000)}'de bitti`
}
