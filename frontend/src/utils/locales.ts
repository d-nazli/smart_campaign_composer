import type { Campaign, LocaleCode, LocaleInfo, MessageContent, StoredContent } from '../types'

/** The source locale until /api/info has answered: every other locale is translated from it. */
export const SOURCE_LOCALE: LocaleCode = 'tr-TR'

/** Used until /api/info has answered (and by tests). */
export const FALLBACK_LOCALES: LocaleInfo[] = [
  { code: SOURCE_LOCALE, name: 'Turkish (Türkiye)', native_name: 'Türkçe (Türkiye)', direction: 'ltr', is_source: true },
]

export const emptyMessage = (): MessageContent => ({ sms_body: '', email_subject: '', email_body: '' })

/** The stored text of every locale of a campaign, by code. */
export function storedByLocale(c: Campaign | null): Record<LocaleCode, StoredContent> {
  const out: Record<LocaleCode, StoredContent> = {}
  if (c?.content) out[c.content.locale ?? c.source_locale ?? SOURCE_LOCALE] = c.content
  for (const [code, sc] of Object.entries(c?.translations ?? {})) out[code] = sc
  return out
}

/** A locale's row from the table, or a plain left-to-right one for a code the table does not list (yet). */
export function localeInfo(code: LocaleCode, all: LocaleInfo[]): LocaleInfo {
  return all.find((l) => l.code === code) ?? { code, name: code, native_name: code, direction: 'ltr', is_source: false }
}

/** The campaign's locales (the source and every locale with a reachable person), source first. */
export function campaignLocales(c: Campaign | null, all: LocaleInfo[]): LocaleInfo[] {
  const source = c?.source_locale ?? all.find((l) => l.is_source)?.code ?? SOURCE_LOCALE
  const codes = c?.locales?.length ? c.locales : [source]
  return codes.map((code) => localeInfo(code, all))
}

const plain = (c: MessageContent): MessageContent => ({ sms_body: c.sms_body, email_subject: c.email_subject, email_body: c.email_body })

/** Editable drafts for the given locales: the stored text where there is one, else empty. */
export function draftsFrom(c: Campaign | null, codes: LocaleCode[]): Record<LocaleCode, MessageContent> {
  const stored = storedByLocale(c)
  const out: Record<LocaleCode, MessageContent> = {}
  for (const code of codes) out[code] = stored[code] ? plain(stored[code]) : emptyMessage()
  return out
}

const same = (a: MessageContent, b: MessageContent) =>
  a.sms_body === b.sms_body && a.email_subject === b.email_subject && a.email_body === b.email_body

/** The locales whose draft differs from what is stored (a locale with no stored text counts once it is typed in). */
export function changedLocales(c: Campaign | null, drafts: Record<LocaleCode, MessageContent>): LocaleCode[] {
  if (!c?.content) return []
  const stored = storedByLocale(c)
  return Object.keys(drafts).filter((code) => {
    const s = stored[code]
    const d = drafts[code]
    return s ? !same(s, d) : !same(d, emptyMessage())
  })
}

/** What a locale's text is: the source, up to date, made from an older source (stale), missing. */
export type LocaleState = 'source' | 'ok' | 'stale' | 'missing'

export function localeState(c: Campaign | null, code: LocaleCode): LocaleState {
  if (code === (c?.source_locale ?? SOURCE_LOCALE)) return 'source'
  const t = c?.translations?.[code]
  if (!t) return 'missing'
  return t.stale ? 'stale' : 'ok'
}

/** The state as a short label (no sentence). */
export const stateLabel: Record<LocaleState, string> = {
  source: 'Kaynak',
  ok: 'Güncel',
  stale: 'Güncel değil',
  missing: 'Çeviri yok',
}

/** The campaign's translations that are missing or stale. */
export const pendingLocales = (c: Campaign | null): LocaleCode[] => c?.pending_locales ?? []

/** Locales in table order, then any the table does not list. */
function ordered(codes: LocaleCode[], all: LocaleInfo[]): LocaleCode[] {
  const known = all.map((l) => l.code).filter((code) => codes.includes(code))
  return [...known, ...codes.filter((code) => !known.includes(code)).sort()]
}

/** How many reachable people each locale has, in table order (locales with nobody left out). */
export function localeSummary(counts: Record<LocaleCode, number> | undefined, all: LocaleInfo[]): { code: LocaleCode; count: number }[] {
  if (!counts) return []
  return ordered(Object.keys(counts).filter((code) => (counts[code] ?? 0) > 0), all).map((code) => ({ code, count: counts[code] }))
}

/** "tr-TR 171 · en-GB 72": a number per locale, in table order, locales without any left out. */
export function countsLine(counts: Record<LocaleCode, number> | undefined, all: LocaleInfo[]): string {
  return localeSummary(counts, all)
    .map((l) => `${l.code} ${l.count}`)
    .join(' · ')
}

/**
 * The same for sending: what was sent to each locale. While messages are still
 * waiting it reads "tr-TR 120/171", once everything has been handled "tr-TR 171".
 */
export function sentLine(
  progress: Record<LocaleCode, { total: number; sent: number }> | undefined,
  all: LocaleInfo[],
  done: boolean,
): string {
  if (!progress) return ''
  return ordered(Object.keys(progress).filter((code) => (progress[code]?.total ?? 0) > 0), all)
    .map((code) => {
      const p = progress[code]
      return `${code} ${done ? p.sent : `${p.sent}/${p.total}`}`
    })
    .join(' · ')
}
