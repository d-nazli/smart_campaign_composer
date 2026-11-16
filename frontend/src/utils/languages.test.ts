import { describe, expect, it } from 'vitest'
import type { Campaign, StoredContent } from '../types'
import { FALLBACK_LANGUAGES, changedLanguages, draftsFrom, hasAllLanguages, languageSummary, storedByLanguage } from './languages'

const stored = (language: string, sms: string): StoredContent => ({
  language,
  sms_body: sms,
  email_subject: 's',
  email_body: 'b',
  source: 'llm',
  report: { ok: true, issues: [], sms_length: 1, subject_length: 1 },
  updated_at: '',
})

const campaign = (withDe: boolean): Campaign => ({
  id: 1,
  brief: 'b',
  status: 'content_ready',
  content: stored('tr', 'merhaba'),
  translations: { en: stored('en', 'hello'), ...(withDe ? { de: stored('de', 'hallo') } : {}) },
  created_at: '',
})

describe('languages', () => {
  it('collects the stored text of every language', () => {
    expect(Object.keys(storedByLanguage(campaign(true))).sort()).toEqual(['de', 'en', 'tr'])
    expect(storedByLanguage(null)).toEqual({})
  })

  it('knows when a language is missing', () => {
    expect(hasAllLanguages(campaign(true), FALLBACK_LANGUAGES)).toBe(true)
    expect(hasAllLanguages(campaign(false), FALLBACK_LANGUAGES)).toBe(false)
    expect(hasAllLanguages(null, FALLBACK_LANGUAGES)).toBe(false)
  })

  it('makes editable drafts, empty for a language without text', () => {
    const d = draftsFrom(campaign(false), FALLBACK_LANGUAGES)
    expect(d.en.sms_body).toBe('hello')
    expect(d.de).toEqual({ sms_body: '', email_subject: '', email_body: '' })
    expect(Object.keys(d)).toEqual(['tr', 'en', 'de'])
    expect(draftsFrom(null, FALLBACK_LANGUAGES).tr.sms_body).toBe('')
  })

  it('reports exactly the languages that were edited', () => {
    const c = campaign(true)
    const d = draftsFrom(c, FALLBACK_LANGUAGES)
    expect(changedLanguages(c, d)).toEqual([])
    d.de = { ...d.de, sms_body: 'Hallo Welt' }
    expect(changedLanguages(c, d)).toEqual(['de'])
    // A campaign without stored text has nothing to compare with: no "unsaved" state.
    expect(changedLanguages(null, d)).toEqual([])
  })

  it('summarises the audience by language, in language order, leaving out empty ones', () => {
    expect(languageSummary({ de: 3, tr: 10, en: 0 }, FALLBACK_LANGUAGES)).toEqual([
      { code: 'tr', display: 'Türkçe', count: 10 },
      { code: 'de', display: 'Deutsch', count: 3 },
    ])
    expect(languageSummary(undefined, FALLBACK_LANGUAGES)).toEqual([])
  })
})
