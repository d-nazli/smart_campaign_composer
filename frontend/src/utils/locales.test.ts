import { describe, expect, it } from 'vitest'
import type { Campaign, LocaleInfo, StoredContent } from '../types'
import {
  campaignLocales,
  changedLocales,
  countsLine,
  draftsFrom,
  localeInfo,
  localeState,
  localeSummary,
  pendingLocales,
  sentLine,
  storedByLocale,
} from './locales'

const L = (code: string, extra: Partial<LocaleInfo> = {}): LocaleInfo => ({ code, name: code, native_name: code, direction: 'ltr', is_source: false, ...extra })
const ALL: LocaleInfo[] = [L('tr-TR', { is_source: true }), L('ar-SA', { direction: 'rtl' }), L('de-DE'), L('en-GB'), L('fr-FR')]

const stored = (locale: string, sms: string, extra: Partial<StoredContent> = {}): StoredContent => ({
  locale,
  sms_body: sms,
  email_subject: 's',
  email_body: 'b',
  source: 'translated',
  version: 1,
  source_version: 1,
  stale: false,
  report: { ok: true, issues: [], sms_length: 1, subject_length: 1 },
  updated_at: '',
  ...extra,
})

const campaign = (): Campaign => ({
  id: 1,
  brief: 'b',
  status: 'content_ready',
  source_locale: 'tr-TR',
  locales: ['tr-TR', 'ar-SA', 'de-DE', 'en-GB'],
  content: stored('tr-TR', 'merhaba', { source: 'llm', version: 2, source_version: undefined }),
  translations: {
    'en-GB': stored('en-GB', 'hello'),
    'de-DE': stored('de-DE', 'hallo', { stale: true }),
    'ar-SA': stored('ar-SA', 'مرحبا', { stale: true }),
  },
  pending_locales: ['ar-SA', 'de-DE'],
  created_at: '',
})

describe('locales', () => {
  it('collects the stored text of every locale', () => {
    expect(Object.keys(storedByLocale(campaign())).sort()).toEqual(['ar-SA', 'de-DE', 'en-GB', 'tr-TR'])
    expect(storedByLocale(null)).toEqual({})
  })

  it("lists the campaign's locales, the source first; before a campaign only the source", () => {
    expect(campaignLocales(campaign(), ALL).map((l) => l.code)).toEqual(['tr-TR', 'ar-SA', 'de-DE', 'en-GB'])
    expect(campaignLocales(campaign(), ALL)[1].direction).toBe('rtl')
    expect(campaignLocales(null, ALL).map((l) => l.code)).toEqual(['tr-TR'])
    // A locale the screen does not know yet (a row added after /api/info) still shows.
    expect(localeInfo('fr-CA', ALL)).toMatchObject({ code: 'fr-CA', direction: 'ltr', is_source: false })
  })

  it('names the state of each locale', () => {
    const c = campaign()
    expect(localeState(c, 'tr-TR')).toBe('source')
    expect(localeState(c, 'en-GB')).toBe('ok')
    expect(localeState(c, 'de-DE')).toBe('stale')
    expect(localeState(c, 'ar-SA')).toBe('stale')
    expect(localeState(c, 'fr-FR')).toBe('missing')
    expect(pendingLocales(c)).toEqual(['ar-SA', 'de-DE'])
  })

  it('makes editable drafts and reports exactly the locales that were edited', () => {
    const c = campaign()
    const d = draftsFrom(c, ['tr-TR', 'en-GB', 'fr-FR'])
    expect(d['en-GB'].sms_body).toBe('hello')
    expect(d['fr-FR']).toEqual({ sms_body: '', email_subject: '', email_body: '' })
    expect(changedLocales(c, d)).toEqual([])
    d['en-GB'] = { ...d['en-GB'], sms_body: 'Hello there' }
    d['fr-FR'] = { ...d['fr-FR'], sms_body: 'Bonjour' }
    expect(changedLocales(c, d)).toEqual(['en-GB', 'fr-FR'])
    // A campaign without stored text has nothing to compare with: no "unsaved" state.
    expect(changedLocales(null, d)).toEqual([])
  })

  it('summarises counts in table order, leaving out locales with nobody', () => {
    expect(localeSummary({ 'de-DE': 3, 'tr-TR': 10, 'en-GB': 0, 'xx-YY': 1 }, ALL)).toEqual([
      { code: 'tr-TR', count: 10 },
      { code: 'de-DE', count: 3 },
      { code: 'xx-YY', count: 1 },
    ])
    expect(countsLine({ 'en-GB': 72, 'tr-TR': 171 }, ALL)).toBe('tr-TR 171 · en-GB 72')
    expect(countsLine(undefined, ALL)).toBe('')
  })

  it('shows sent/total while messages wait, and what was sent once done', () => {
    const p = { 'tr-TR': { total: 171, sent: 171 }, 'ar-SA': { total: 42, sent: 0 }, 'en-GB': { total: 0, sent: 0 } }
    expect(sentLine(p, ALL, false)).toBe('tr-TR 171/171 · ar-SA 0/42')
    expect(sentLine(p, ALL, true)).toBe('tr-TR 171 · ar-SA 0')
    expect(sentLine(undefined, ALL, true)).toBe('')
  })
})
