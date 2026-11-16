import type { Campaign, LanguageCode, LanguageInfo, MessageContent, StoredContent } from '../types'

/** The reference language: the others are written to say the same thing as it does. */
export const DEFAULT_LANGUAGE: LanguageCode = 'tr'

/** Used until /api/info has answered (and by tests). */
export const FALLBACK_LANGUAGES: LanguageInfo[] = [
  { code: 'tr', name: 'Turkish', display: 'Türkçe' },
  { code: 'en', name: 'English', display: 'English' },
  { code: 'de', name: 'German', display: 'Deutsch' },
]

export const emptyMessage = (): MessageContent => ({ sms_body: '', email_subject: '', email_body: '' })

/** The stored text of every language of a campaign, by code. */
export function storedByLanguage(c: Campaign | null): Record<LanguageCode, StoredContent> {
  const out: Record<LanguageCode, StoredContent> = {}
  if (c?.content) out[c.content.language ?? DEFAULT_LANGUAGE] = c.content
  for (const [code, sc] of Object.entries(c?.translations ?? {})) out[code] = sc
  return out
}

const plain = (c: MessageContent): MessageContent => ({ sms_body: c.sms_body, email_subject: c.email_subject, email_body: c.email_body })

/** Editable drafts for every language: the stored text where there is one, else empty. */
export function draftsFrom(c: Campaign | null, languages: LanguageInfo[]): Record<LanguageCode, MessageContent> {
  const stored = storedByLanguage(c)
  const out: Record<LanguageCode, MessageContent> = {}
  for (const l of languages) out[l.code] = stored[l.code] ? plain(stored[l.code]) : emptyMessage()
  return out
}

/** The languages whose draft differs from what is stored. */
export function changedLanguages(c: Campaign | null, drafts: Record<LanguageCode, MessageContent>): LanguageCode[] {
  const stored = storedByLanguage(c)
  return Object.keys(drafts).filter((code) => {
    const s = stored[code]
    const d = drafts[code]
    return !!s && (s.sms_body !== d.sms_body || s.email_subject !== d.email_subject || s.email_body !== d.email_body)
  })
}

/** Every language has some stored text. */
export function hasAllLanguages(c: Campaign | null, languages: LanguageInfo[]): boolean {
  const stored = storedByLanguage(c)
  return languages.length > 0 && languages.every((l) => !!stored[l.code])
}

/** How many reachable people speak each language, in language order (languages nobody speaks are left out). */
export function languageSummary(
  counts: Record<LanguageCode, number> | undefined,
  languages: LanguageInfo[],
): { code: LanguageCode; display: string; count: number }[] {
  if (!counts) return []
  return languages.filter((l) => (counts[l.code] ?? 0) > 0).map((l) => ({ code: l.code, display: l.display, count: counts[l.code] }))
}

/** "TR 171 · EN 72 · DE 42": a number per language, in language order, languages without any left out. */
export function countsLine(counts: Record<LanguageCode, number> | undefined, languages: LanguageInfo[]): string {
  if (!counts) return ''
  return languages
    .filter((l) => (counts[l.code] ?? 0) > 0)
    .map((l) => `${l.code.toUpperCase()} ${counts[l.code]}`)
    .join(' · ')
}

/**
 * The same for sending: what was sent to each language. While messages are still
 * waiting it reads "TR 120/171", once everything has been handled "TR 171".
 */
export function sentLine(
  langs: Record<LanguageCode, { total: number; sent: number }> | undefined,
  languages: LanguageInfo[],
  done: boolean,
): string {
  if (!langs) return ''
  return languages
    .filter((l) => (langs[l.code]?.total ?? 0) > 0)
    .map((l) => {
      const p = langs[l.code]
      return `${l.code.toUpperCase()} ${done ? p.sent : `${p.sent}/${p.total}`}`
    })
    .join(' · ')
}
