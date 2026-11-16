import { describe, expect, it } from 'vitest'
import type { Campaign } from '../types'
import { funnel, share } from './funnel'
import { mergeFailures, failureText } from './failures'
import { activeField, attemptLabel, fillSample, meterLevel } from './preview'
import { pushSample, ratePerSecond } from './rate'
import { currentStep } from './steps'
import { overallTotals, statusTone } from './dashboard'

const camp = (status: Campaign['status'], segOk = true): Campaign =>
  ({ id: 1, brief: 'b', status, created_at: '', segment: { status: segOk ? 'ok' : 'unsupported', conditions: [], explanation: '' } }) as Campaign
const input = (c: Campaign | null, o: Partial<{ streaming: boolean; hasContent: boolean; unsaved: boolean }> = {}) => ({
  campaign: c,
  streaming: false,
  hasContent: false,
  unsaved: false,
  ...o,
})

describe('currentStep', () => {
  it('starts at the brief and stays there while the audience is not usable', () => {
    expect(currentStep(input(null)).current).toBe('brief')
    // a brief the system could not turn into an audience waits on the audience step, where the reason is shown
    expect(currentStep(input(camp('draft', false)))).toEqual({ current: 'audience', done: ['brief'] })
  })
  it('walks through the flow and marks earlier steps done', () => {
    expect(currentStep(input(camp('segment_ready')))).toEqual({ current: 'audience', done: ['brief'] })
    expect(currentStep(input(camp('segment_ready'), { streaming: true })).current).toBe('messages')
    expect(currentStep(input(camp('content_ready'), { hasContent: true }))).toEqual({
      current: 'approve',
      done: ['brief', 'audience', 'messages'],
    })
    expect(currentStep(input(camp('content_ready'), { hasContent: true, unsaved: true })).current).toBe('messages')
    expect(currentStep(input(camp('sending'))).current).toBe('send')
  })
  it('marks the last step done once completed', () => {
    expect(currentStep(input(camp('completed'))).done).toContain('send')
  })
})

describe('preview helpers', () => {
  it('fills known variables only', () => {
    expect(fillSample('Merhaba {{first_name}}, {{ city }} {{x}}')).toBe('Merhaba Elif, İstanbul {{x}}')
  })
  it('finds the field being written', () => {
    expect(activeField({ sms_body: '', email_subject: '', email_body: '' })).toBe('sms_body')
    expect(activeField({ sms_body: 'a', email_subject: 'b', email_body: '' })).toBe('email_subject')
    expect(activeField({ sms_body: 'a', email_subject: 'b', email_body: 'c' })).toBe('email_body')
  })
  it('colours the counter by how close to the limit', () => {
    expect(meterLevel(50, 100)).toBe('ok')
    expect(meterLevel(90, 100)).toBe('near')
    expect(meterLevel(100, 100)).toBe('near')
    expect(meterLevel(101, 100)).toBe('over')
  })
  it('words the attempt badge', () => {
    expect(attemptLabel({ n: 1, max: 3 })).toBe('üretiliyor 1/3')
    expect(attemptLabel({ n: 2, max: 3 })).toBe('düzeltiliyor 2/3')
  })
})

describe('funnel', () => {
  it('splits the matched people by channel', () => {
    expect(funnel({ matched: 300, sms_reachable: 203, email_reachable: 227, both_reachable: 165 })).toEqual({
      matched: 300,
      sms: 203,
      email: 227,
      both: 165,
      smsOnly: 38,
      emailOnly: 62,
      unreachable: 35,
    })
  })
  it('gives a sliver to small non-zero shares', () => {
    expect(share(0, 100)).toBe(0)
    expect(share(1, 1000)).toBe(2)
    expect(share(50, 100)).toBe(50)
    expect(share(5, 0)).toBe(0)
  })
})

describe('failures, rate, dashboard', () => {
  it('merges channels and sorts by count', () => {
    const m = mergeFailures([
      { channel: 'sms', reason: 'gave_up', count: 2 },
      { channel: 'sms', reason: 'rejected', count: 3 },
      { channel: 'email', reason: 'gave_up', count: 4 },
    ])
    expect(m).toEqual([
      { reason: 'gave_up', count: 6 },
      { reason: 'rejected', count: 3 },
    ])
    expect(failureText('nope')).toBe(failureText('other'))
  })
  it('computes messages per second over the window', () => {
    let s = pushSample([], { at: 0, sent: 0 })
    expect(ratePerSecond(s)).toBe(0)
    s = pushSample(s, { at: 2000, sent: 20 })
    s = pushSample(s, { at: 4000, sent: 40 })
    expect(ratePerSecond(s)).toBe(10)
    s = pushSample(s, { at: 20000, sent: 40 }) // old samples drop out
    expect(s).toHaveLength(1)
    expect(
      ratePerSecond([
        { at: 0, sent: 10 },
        { at: 1000, sent: 5 },
      ]),
    ).toBe(0)
  })
  it('tones and totals', () => {
    expect(statusTone({ status: 'completed' })).toBe('done')
    expect(statusTone({ status: 'sending' })).toBe('sending')
    expect(statusTone({ status: 'draft', segment_status: 'unsupported' })).toBe('attention')
    expect(statusTone({ status: 'content_ready' })).toBe('draft')
    const ch = { retries: 1, rate_limited: 1, total: 5, pending: 1, sent: 3, failed: 1, skipped: 0 }
    const t = overallTotals([{ id: 1, brief: '', status: 'sending', created_at: '', delivery: { sms: ch, email: ch, done: false } }])
    expect(t).toMatchObject({ sent: 6, failed: 2, pending: 2, retries: 2, rateLimited: 2 })
  })
})
