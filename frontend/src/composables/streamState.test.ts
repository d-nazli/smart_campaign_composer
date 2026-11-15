import { describe, expect, it } from 'vitest'
import type { SSEEvent } from '../api/sse'
import { initialState, reduce, startedState, type StreamState } from './streamState'

const ev = (event: string, data: unknown): SSEEvent => ({ event, data })
const run = (events: SSEEvent[], from: StreamState = startedState()) => events.reduce(reduce, from)

const content = { sms_body: 'S', email_subject: 'K', email_body: 'G' }
const report = { ok: true, issues: [], sms_length: 1, subject_length: 1 }

describe('stream reducer', () => {
  it('builds the live draft from deltas and finishes with the validated result', () => {
    const s = run([
      ev('attempt', { n: 1, max: 3 }),
      ev('delta', { field: 'sms_body', text: 'Mer' }),
      ev('delta', { field: 'sms_body', text: 'haba' }),
      ev('delta', { field: 'email_subject', text: 'Konu' }),
    ])
    expect(s.draft).toEqual({ sms_body: 'Merhaba', email_subject: 'Konu', email_body: '' })
    expect(s.phase).toBe('streaming')

    const done = run([ev('validated', { content, report }), ev('done', { status: 'content_ready' })], s)
    expect(done.phase).toBe('done')
    expect(done.result).toEqual({ content, report, source: 'llm' })
    expect(done.draft).toEqual(content)
  })

  it('discards the rejected draft when a new attempt starts and remembers why', () => {
    const s = run([
      ev('attempt', { n: 1, max: 3 }),
      ev('delta', { field: 'sms_body', text: 'çok uzun' }),
      ev('retry', { next: 2, problems: ['SMS en fazla 160 karakter olmalı'] }),
      ev('attempt', { n: 2, max: 3 }),
    ])
    expect(s.draft.sms_body).toBe('')
    expect(s.attempt).toEqual({ n: 2, max: 3 })
    expect(s.retryProblems).toEqual(['SMS en fazla 160 karakter olmalı'])
  })

  it('marks fallback content and clears the retry reasons on success', () => {
    const s = run([ev('retry', { next: 2, problems: ['x'] }), ev('fallback', { reason: 'invalid_output', content, report })])
    expect(s.fallbackReason).toBe('invalid_output')
    expect(s.result?.source).toBe('fallback')
  })

  it('records errors and ignores unknown fields or events', () => {
    const s = run([ev('delta', { field: 'bogus', text: 'x' }), ev('mystery', {}), ev('error', { code: 'llm_misconfigured', message: 'anahtar' })])
    expect(s.draft).toEqual(initialState().draft)
    expect(s.phase).toBe('error')
    expect(s.error).toEqual({ code: 'llm_misconfigured', message: 'anahtar' })
  })
})
