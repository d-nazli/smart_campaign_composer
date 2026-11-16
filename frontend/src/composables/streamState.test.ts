import { describe, expect, it } from 'vitest'
import type { SSEEvent } from '../api/sse'
import { allResults, emptyContent, initialState, reduce, retryNotes, startedState, type StreamState } from './streamState'

const ev = (event: string, data: unknown): SSEEvent => ({ event, data })
const run = (events: SSEEvent[], from: StreamState = startedState()) => events.reduce(reduce, from)

const content = { sms_body: 'S', email_subject: 'K', email_body: 'G' }
const report = { ok: true, issues: [], sms_length: 1, subject_length: 1 }

describe('stream reducer', () => {
  it('builds the live draft from deltas and finishes with the validated result', () => {
    const s = run([
      ev('attempt', { language: 'tr', n: 1, max: 3 }),
      ev('delta', { language: 'tr', field: 'sms_body', text: 'Mer' }),
      ev('delta', { language: 'tr', field: 'sms_body', text: 'haba' }),
      ev('delta', { language: 'tr', field: 'email_subject', text: 'Konu' }),
    ])
    expect(s.langs.tr.draft).toEqual({ sms_body: 'Merhaba', email_subject: 'Konu', email_body: '' })
    expect(s.phase).toBe('streaming')

    const done = run([ev('validated', { language: 'tr', content, report }), ev('done', { status: 'content_ready' })], s)
    expect(done.phase).toBe('done')
    expect(done.langs.tr.result).toEqual({ content, report, source: 'llm' })
    expect(done.langs.tr.draft).toEqual(content)
  })

  it('keeps every language apart, even when their events interleave', () => {
    const s = run([
      ev('attempt', { language: 'tr', n: 1, max: 3 }),
      ev('attempt', { language: 'en', n: 1, max: 3 }),
      ev('delta', { language: 'en', field: 'sms_body', text: 'Hi ' }),
      ev('delta', { language: 'tr', field: 'sms_body', text: 'Merhaba ' }),
      ev('delta', { language: 'en', field: 'sms_body', text: 'there' }),
    ])
    expect(s.langs.tr.draft.sms_body).toBe('Merhaba ')
    expect(s.langs.en.draft.sms_body).toBe('Hi there')
    expect(s.langs.de).toBeUndefined()
  })

  it('treats an event without a language as the reference language (tr)', () => {
    const s = run([ev('delta', { field: 'sms_body', text: 'x' })])
    expect(s.langs.tr.draft.sms_body).toBe('x')
  })

  it('discards the rejected draft when a new attempt starts and remembers why, per language', () => {
    const s = run([
      ev('attempt', { language: 'en', n: 1, max: 3 }),
      ev('delta', { language: 'en', field: 'sms_body', text: 'too long' }),
      ev('retry', { language: 'en', next: 2, problems: ['SMS en fazla 160 karakter olmalı'] }),
      ev('attempt', { language: 'en', n: 2, max: 3 }),
    ])
    expect(s.langs.en.draft.sms_body).toBe('')
    expect(s.langs.en.attempt).toEqual({ n: 2, max: 3 })
    expect(s.langs.en.retryProblems).toEqual(['SMS en fazla 160 karakter olmalı'])
    expect(retryNotes(s)).toEqual([{ language: 'en', problems: ['SMS en fazla 160 karakter olmalı'] }])
    expect(retryNotes(run([ev('validated', { language: 'en', content, report })], s))).toEqual([])
  })

  it('marks fallback content for every language and clears the retry reasons', () => {
    const s = run([
      ev('retry', { language: 'tr', next: 2, problems: ['x'] }),
      ev('fallback', { language: 'tr', reason: 'invalid_output', content, report }),
      ev('fallback', { language: 'en', reason: 'invalid_output', content, report }),
      ev('fallback', { language: 'de', reason: 'invalid_output', content, report }),
    ])
    expect(s.fallbackReason).toBe('invalid_output')
    expect(s.langs.en.result?.source).toBe('fallback')
    expect(retryNotes(s)).toEqual([])
    expect(allResults(s, ['tr', 'en', 'de'])).toBe(true)
  })

  it('is complete only when every language has its result', () => {
    const s = run([ev('validated', { language: 'tr', content, report }), ev('validated', { language: 'en', content, report })])
    expect(allResults(s, ['tr', 'en', 'de'])).toBe(false)
    expect(allResults(run([ev('validated', { language: 'de', content, report })], s), ['tr', 'en', 'de'])).toBe(true)
    expect(allResults(initialState(), [])).toBe(false)
  })

  it('records errors and ignores unknown fields or events', () => {
    const s = run([ev('delta', { field: 'bogus', text: 'x' }), ev('mystery', {}), ev('error', { code: 'llm_misconfigured', message: 'anahtar' })])
    expect(s.langs).toEqual({})
    expect(emptyContent()).toEqual({ sms_body: '', email_subject: '', email_body: '' })
    expect(s.phase).toBe('error')
    expect(s.error).toEqual({ code: 'llm_misconfigured', message: 'anahtar' })
  })
})
