import { describe, expect, it } from 'vitest'
import type { Campaign, ChannelProgress } from '../types'
import { approvalChange, channelPercent, describeChannel, shouldPoll } from './delivery'

const p = (o: Partial<ChannelProgress> = {}): ChannelProgress => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })

const campaign = (status: Campaign['status'], delivery?: Campaign['delivery']): Campaign => ({
  id: 1,
  brief: 'b',
  status,
  delivery,
  created_at: '2026-01-01T00:00:00Z',
})

describe('channelPercent', () => {
  it('counts everything that has reached a final state', () => {
    expect(channelPercent(p({ total: 200, sent: 50, failed: 25, skipped: 25, pending: 100 }))).toBe(50)
  })
  it('is 100 for a channel with nobody to send to, not NaN', () => {
    expect(channelPercent(p())).toBe(100)
  })
  it('never reports 100 while something is pending, even when rounding would', () => {
    expect(channelPercent(p({ total: 1000, sent: 999, pending: 1 }))).toBe(99)
  })
  it('is 100 when everything is final', () => {
    expect(channelPercent(p({ total: 3, sent: 2, failed: 1 }))).toBe(100)
  })
})

describe('describeChannel', () => {
  it('says what happened to everyone, failed ones included', () => {
    expect(describeChannel(p({ total: 10, sent: 6, failed: 2, skipped: 1, pending: 1 }))).toBe(
      '6 gönderildi · 2 başarısız · 1 atlandı · 1 bekliyor',
    )
  })
  it('leaves out zero counts', () => {
    expect(describeChannel(p({ total: 4, sent: 4 }))).toBe('4 gönderildi')
  })
  it('says so when there is nobody to send to', () => {
    expect(describeChannel(p())).toBe('Alıcı yok')
  })
})

describe('shouldPoll', () => {
  it('keeps polling while the campaign is being sent', () => {
    expect(shouldPoll(campaign('approved', { sms: p({ total: 5, pending: 5 }), email: p(), done: false }))).toBe(true)
    expect(shouldPoll(campaign('sending', { sms: p({ total: 5, pending: 2, sent: 3 }), email: p(), done: false }))).toBe(true)
  })
  it('keeps polling right after approval, before the first progress is known', () => {
    expect(shouldPoll(campaign('approved'))).toBe(true)
  })
  it('stops when everything is final and the campaign is completed', () => {
    expect(shouldPoll(campaign('completed', { sms: p({ total: 5, sent: 5 }), email: p(), done: true }))).toBe(false)
  })
  it('keeps polling until the server marks the campaign completed, even when all rows are final', () => {
    expect(shouldPoll(campaign('sending', { sms: p({ total: 5, sent: 5 }), email: p(), done: true }))).toBe(true)
  })
  it('does not poll campaigns that were never approved', () => {
    expect(shouldPoll(campaign('content_ready'))).toBe(false)
    expect(shouldPoll(campaign('draft'))).toBe(false)
  })
})

describe('approvalChange', () => {
  const same = { shown: { sms: 1677, email: 3794 }, queued: { sms: 1677, email: 3794 } }
  it('is silent after a normal approval (nothing changed)', () => {
    expect(approvalChange(same)).toBeNull()
  })
  it('says how many SMS recipients changed, with both numbers', () => {
    expect(approvalChange({ ...same, queued: { sms: 1650, email: 3794 } })).toBe('Onaydan sonra 27 SMS alıcısı değişti (1.677 → 1.650).')
  })
  it('names both channels when both changed', () => {
    expect(approvalChange({ ...same, queued: { sms: 1650, email: 3789 } })).toBe(
      'Onaydan sonra 27 SMS alıcısı (1.677 → 1.650) ve 5 e-posta alıcısı (3.794 → 3.789) değişti.',
    )
  })
  it('counts a growth too', () => {
    expect(approvalChange({ ...same, queued: { sms: 1700, email: 3794 } })).toBe('Onaydan sonra 23 SMS alıcısı değişti (1.677 → 1.700).')
  })
  it('is silent before anything is queued', () => {
    expect(approvalChange(undefined)).toBeNull()
  })
})
