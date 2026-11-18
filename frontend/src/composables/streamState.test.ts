import { describe, expect, it } from 'vitest'
import type { SSEEvent } from '../api/sse'
import { allResults, emptyContent, failedLocales, initialState, reduce, retryNotes, startedState, type StreamState } from './streamState'

const ev = (event: string, data: unknown): SSEEvent => ({ event, data })
const run = (events: SSEEvent[], from: StreamState = startedState()) => events.reduce(reduce, from)

const content = { sms_body: 'S', email_subject: 'K', email_body: 'G' }
const report = { ok: true, issues: [], sms_length: 1, subject_length: 1 }

describe('stream reducer', () => {
  it('builds the live draft from deltas and finishes with the validated result', () => {
    const s = run([
      ev('attempt', { locale: 'tr-TR', n: 1, max: 3 }),
      ev('delta', { locale: 'tr-TR', field: 'sms_body', text: 'Mer' }),
      ev('delta', { locale: 'tr-TR', field: 'sms_body', text: 'haba' }),
      ev('delta', { locale: 'tr-TR', field: 'email_subject', text: 'Konu' }),
    ])
    expect(s.locales['tr-TR'].draft).toEqual({ sms_body: 'Merhaba', email_subject: 'Konu', email_body: '' })
    expect(s.phase).toBe('streaming')

    const done = run([ev('validated', { locale: 'tr-TR', content, report, source: 'llm' }), ev('done', { status: 'content_ready' })], s)
    expect(done.phase).toBe('done')
    expect(done.locales['tr-TR'].result).toEqual({ content, report, source: 'llm' })
    expect(done.locales['tr-TR'].draft).toEqual(content)
  })

  it('keeps every locale apart, even when their events interleave', () => {
    const s = run([
      ev('attempt', { locale: 'en-GB', n: 1, max: 3 }),
      ev('attempt', { locale: 'ar-SA', n: 1, max: 3 }),
      ev('delta', { locale: 'en-GB', field: 'sms_body', text: 'Hi ' }),
      ev('delta', { locale: 'ar-SA', field: 'sms_body', text: 'مرحبا ' }),
      ev('delta', { locale: 'en-GB', field: 'sms_body', text: 'there' }),
    ])
    expect(s.locales['ar-SA'].draft.sms_body).toBe('مرحبا ')
    expect(s.locales['en-GB'].draft.sms_body).toBe('Hi there')
    expect(s.locales['en-US']).toBeUndefined()
  })

  it('keeps the source of a translation', () => {
    const s = run([ev('validated', { locale: 'fr-FR', content, report, source: 'translated' })])
    expect(s.locales['fr-FR'].result?.source).toBe('translated')
  })

  it('treats an event without a locale as the source (tr-TR)', () => {
    const s = run([ev('delta', { field: 'sms_body', text: 'x' })])
    expect(s.locales['tr-TR'].draft.sms_body).toBe('x')
  })

  it('discards the rejected draft when a new attempt starts and remembers why, per locale', () => {
    const s = run([
      ev('attempt', { locale: 'en-GB', n: 1, max: 3 }),
      ev('delta', { locale: 'en-GB', field: 'sms_body', text: 'too long' }),
      ev('retry', { locale: 'en-GB', next: 2, problems: ['SMS en fazla 160 karakter olmalı'] }),
      ev('attempt', { locale: 'en-GB', n: 2, max: 3 }),
    ])
    expect(s.locales['en-GB'].draft.sms_body).toBe('')
    expect(s.locales['en-GB'].attempt).toEqual({ n: 2, max: 3 })
    expect(retryNotes(s)).toEqual([{ locale: 'en-GB', problems: ['SMS en fazla 160 karakter olmalı'] }])
    expect(retryNotes(run([ev('validated', { locale: 'en-GB', content, report })], s))).toEqual([])
  })

  it('marks the fallback source text and clears the retry reasons', () => {
    const s = run([ev('retry', { locale: 'tr-TR', next: 2, problems: ['x'] }), ev('fallback', { locale: 'tr-TR', reason: 'invalid_output', content, report })])
    expect(s.fallbackReason).toBe('invalid_output')
    expect(s.locales['tr-TR'].result?.source).toBe('fallback')
    expect(retryNotes(s)).toEqual([])
  })

  it('records a translation that could not be made; it counts as finished', () => {
    const s = run([
      ev('validated', { locale: 'tr-TR', content, report }),
      ev('attempt', { locale: 'it-IT', n: 1, max: 3 }),
      ev('retry', { locale: 'it-IT', next: 2, problems: ['x'] }),
      ev('failed', { locale: 'it-IT', reason: 'invalid_output' }),
    ])
    expect(s.locales['it-IT'].failed).toBe('invalid_output')
    expect(s.locales['it-IT'].result).toBeNull()
    expect(failedLocales(s)).toEqual(['it-IT'])
    expect(retryNotes(s)).toEqual([])
    expect(allResults(s, ['tr-TR', 'it-IT'])).toBe(true)
  })

  it('is complete only when every locale has finished', () => {
    const s = run([ev('validated', { locale: 'tr-TR', content, report }), ev('validated', { locale: 'en-GB', content, report })])
    expect(allResults(s, ['tr-TR', 'en-GB', 'de-DE'])).toBe(false)
    expect(allResults(run([ev('validated', { locale: 'de-DE', content, report })], s), ['tr-TR', 'en-GB', 'de-DE'])).toBe(true)
    expect(allResults(initialState(), [])).toBe(false)
  })

  it('records errors and ignores unknown fields or events', () => {
    const s = run([ev('delta', { field: 'bogus', text: 'x' }), ev('mystery', {}), ev('error', { code: 'llm_misconfigured', message: 'anahtar' })])
    expect(s.locales).toEqual({})
    expect(emptyContent()).toEqual({ sms_body: '', email_subject: '', email_body: '' })
    expect(s.phase).toBe('error')
    expect(s.error).toEqual({ code: 'llm_misconfigured', message: 'anahtar' })
  })
})
