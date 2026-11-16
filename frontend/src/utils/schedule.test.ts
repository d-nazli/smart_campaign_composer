import { describe, expect, it } from 'vitest'
import type { Offer } from '../types'
import { checkSchedule, dayText, fromLocalInput, offerLastMoment, scheduleBounds, toLocalInput, untilText } from './schedule'

const now = new Date('2026-11-16T09:00:00Z')
const at = (ms: number) => new Date(now.getTime() + ms)
const MIN = 60 * 1000
const HOUR = 60 * MIN
const DAY = 24 * HOUR
const offer = (expires_at?: string): Offer => ({ kind: 'percent', value: 30, code: '', expires_at })

describe('the last moment of an offer', () => {
  it('is the end of that day in Istanbul (UTC+3), not in the viewer zone', () => {
    expect(offerLastMoment('2026-12-31')?.toISOString()).toBe('2026-12-31T20:59:59.999Z')
    expect(offerLastMoment('2026-03-01')?.toISOString()).toBe('2026-03-01T20:59:59.999Z')
  })
  it('does not exist for an offer without a last day', () => {
    expect(offerLastMoment(undefined)).toBeNull()
    expect(offerLastMoment('')).toBeNull()
    expect(offerLastMoment('31.12.2026')).toBeNull()
  })
})

describe('checkSchedule', () => {
  it('asks for a time', () => {
    expect(checkSchedule(null, offer(), now)).toMatch(/seçin/)
  })
  it('refuses a time that is too close or in the past', () => {
    expect(checkSchedule(at(-HOUR), offer(), now)).toMatch(/en az 1 dakika/)
    expect(checkSchedule(at(30 * 1000), offer(), now)).toMatch(/en az 1 dakika/)
    expect(checkSchedule(at(MIN), offer(), now)).toBeNull()
  })
  it('refuses more than 30 days ahead', () => {
    expect(checkSchedule(at(30 * DAY), offer(), now)).toBeNull()
    expect(checkSchedule(at(30 * DAY + MIN), offer(), now)).toMatch(/30 gün/)
  })
  it('refuses a time after the offer ends, naming its last day, but allows the last day itself', () => {
    const o = offer('2026-11-20')
    const lastMoment = offerLastMoment('2026-11-20')!
    expect(checkSchedule(new Date(lastMoment.getTime() - HOUR), o, now)).toBeNull()
    expect(checkSchedule(new Date(lastMoment.getTime() + HOUR), o, now)).toMatch(/20\.11\.2026/)
  })
  it('an offer without a last day only has the 30-day window', () => {
    expect(checkSchedule(at(10 * DAY), offer(), now)).toBeNull()
  })
})

describe('scheduleBounds', () => {
  it('is limited by the offer when it ends sooner than 30 days', () => {
    const b = scheduleBounds(offer('2026-11-20'), now)
    expect(b.limitedByOffer).toBe(true)
    expect(b.latest.toISOString()).toBe('2026-11-20T20:59:59.999Z')
    expect(b.earliest.getTime()).toBe(now.getTime() + MIN)
  })
  it('is limited by the 30 days when the offer lasts longer or has no end', () => {
    expect(scheduleBounds(offer('2027-12-31'), now).limitedByOffer).toBe(false)
    expect(scheduleBounds(offer(), now).latest.getTime()).toBe(now.getTime() + 30 * DAY)
    expect(scheduleBounds(undefined, now).limitedByOffer).toBe(false)
  })
})

describe('datetime-local values', () => {
  it('round-trip in the viewer zone', () => {
    const d = new Date(2026, 10, 16, 14, 5)
    expect(toLocalInput(d)).toBe('2026-11-16T14:05')
    expect(fromLocalInput('2026-11-16T14:05')?.getTime()).toBe(d.getTime())
  })
  it('reject empty or broken values', () => {
    expect(fromLocalInput('')).toBeNull()
    expect(fromLocalInput('yarın')).toBeNull()
  })
  it('writes a day as dd.mm.yyyy', () => {
    expect(dayText('2026-12-31')).toBe('31.12.2026')
  })
})

describe('untilText', () => {
  it('counts down in the largest sensible units', () => {
    expect(untilText(at(5 * MIN), now)).toBe('5 dk sonra')
    expect(untilText(at(2 * HOUR), now)).toBe('2 sa sonra')
    expect(untilText(at(2 * HOUR + 5 * MIN), now)).toBe('2 sa 5 dk sonra')
    expect(untilText(at(3 * DAY + 4 * HOUR), now)).toBe('3 gün 4 sa sonra')
  })
  it('says it is starting when the time has come', () => {
    expect(untilText(at(0), now)).toBe('şimdi başlıyor')
    expect(untilText(at(-HOUR), now)).toBe('şimdi başlıyor')
  })
})
