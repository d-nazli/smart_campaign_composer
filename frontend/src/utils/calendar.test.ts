import { describe, expect, it } from 'vitest'
import {
  clampDate,
  dayDisabled,
  displayValue,
  HOURS,
  longValue,
  MINUTES,
  momentDisabled,
  monthGrid,
  monthLabel,
  sameDay,
  shiftMonth,
  withDay,
  withTime,
} from './calendar'

const d = (y: number, m: number, day: number, h = 0, min = 0) => new Date(y, m - 1, day, h, min)

describe('monthGrid', () => {
  it('always has six weeks, Monday first, so the popup keeps its height', () => {
    for (const [y, m] of [[2026, 1], [2026, 10], [2026, 11], [2027, 1]]) {
      const g = monthGrid(y, m)
      expect(g).toHaveLength(42)
      expect(g[0].date.getDay()).toBe(1) // Monday
    }
  })

  it('starts on the Monday before the 1st (1 November 2026 is a Sunday)', () => {
    const g = monthGrid(2026, 10)
    expect(sameDay(g[0].date, d(2026, 10, 26))).toBe(true)
    expect(g[0].inMonth).toBe(false)
    expect(sameDay(g[6].date, d(2026, 11, 1))).toBe(true)
    expect(g[6].inMonth).toBe(true)
  })

  it('marks exactly the days of the month as in the month', () => {
    expect(monthGrid(2026, 10).filter((c) => c.inMonth)).toHaveLength(30)
    expect(monthGrid(2028, 1).filter((c) => c.inMonth)).toHaveLength(29) // leap year
  })

  it('starts on the 1st itself when it is a Monday', () => {
    const g = monthGrid(2026, 5) // 1 June 2026 is a Monday
    expect(sameDay(g[0].date, d(2026, 6, 1))).toBe(true)
    expect(g[0].inMonth).toBe(true)
  })
})

describe('labels', () => {
  it('names the month in Turkish', () => {
    expect(monthLabel(2026, 10)).toBe('Kasım 2026')
    expect(monthLabel(2027, 0)).toBe('Ocak 2027')
  })
  it('writes a moment as dd.mm.yyyy hh:mm', () => {
    expect(displayValue(d(2026, 3, 5, 9, 7))).toBe('05.03.2026 09:07')
  })
  it('moves between months across a year', () => {
    expect(shiftMonth(2026, 11, 1)).toEqual({ year: 2027, month: 0 })
    expect(shiftMonth(2026, 0, -1)).toEqual({ year: 2025, month: 11 })
  })
})

describe('what can be chosen', () => {
  const min = d(2026, 11, 16, 14, 30)
  const max = d(2026, 11, 20, 12, 0)

  it('allows the whole of the first and last day that have any time inside the range', () => {
    expect(dayDisabled(d(2026, 11, 16), min, max)).toBe(false) // the evening of the 16th is after min
    expect(dayDisabled(d(2026, 11, 20), min, max)).toBe(false) // the morning of the 20th is before max
  })
  it('refuses days outside the range', () => {
    expect(dayDisabled(d(2026, 11, 15), min, max)).toBe(true)
    expect(dayDisabled(d(2026, 11, 21), min, max)).toBe(true)
  })
  it('has no limit where there is no bound', () => {
    expect(dayDisabled(d(2030, 1, 1), min, null)).toBe(false)
    expect(dayDisabled(d(2000, 1, 1), null, max)).toBe(false)
    expect(dayDisabled(d(2000, 1, 1), null, null)).toBe(false)
  })
  it('refuses a moment outside the range, exactly at the bounds', () => {
    expect(momentDisabled(d(2026, 11, 16, 14, 29), min, max)).toBe(true)
    expect(momentDisabled(d(2026, 11, 16, 14, 30), min, max)).toBe(false)
    expect(momentDisabled(d(2026, 11, 20, 12, 0), min, max)).toBe(false)
    expect(momentDisabled(d(2026, 11, 20, 12, 1), min, max)).toBe(true)
  })
})

describe('choosing', () => {
  const min = d(2026, 11, 16, 14, 30)
  const max = d(2026, 11, 20, 12, 0)

  it('moves a time into the range', () => {
    expect(clampDate(d(2026, 11, 10), min, max)).toEqual(min)
    expect(clampDate(d(2026, 12, 1), min, max)).toEqual(max)
    expect(clampDate(d(2026, 11, 18, 8), min, max)).toEqual(d(2026, 11, 18, 8))
  })

  it('keeps the time when another day is chosen, 09:00 when there was none', () => {
    expect(withDay(d(2026, 11, 17, 10, 15), d(2026, 11, 19), min, max)).toEqual(d(2026, 11, 19, 10, 15))
    expect(withDay(null, d(2026, 11, 18), min, max)).toEqual(d(2026, 11, 18, 9, 0))
  })

  it('pulls the time inside the range when the chosen day cuts it off', () => {
    // 09:00 on the first day is before min (14:30): the closest allowed moment is min itself
    expect(withDay(null, d(2026, 11, 16), min, max)).toEqual(min)
    // 18:00 on the last day is after max (12:00)
    expect(withDay(d(2026, 11, 18, 18, 0), d(2026, 11, 20), min, max)).toEqual(max)
  })

  it('changes the time of the current day, or of the fallback day when nothing is chosen yet', () => {
    expect(withTime(d(2026, 11, 18, 10, 0), d(2026, 11, 17), 15, 45, min, max)).toEqual(d(2026, 11, 18, 15, 45))
    expect(withTime(null, d(2026, 11, 17), 8, 5, min, max)).toEqual(d(2026, 11, 17, 8, 5))
  })
})

describe('time choices', () => {
  it('offers every hour and every minute', () => {
    expect(HOURS).toHaveLength(24)
    expect([HOURS[0], HOURS[23]]).toEqual([0, 23])
    expect(MINUTES).toHaveLength(60)
    expect(MINUTES).toEqual(Array.from({ length: 60 }, (_, i) => i))
  })
})

describe('longValue', () => {
  it('writes the day and the month name, then the time', () => {
    expect(longValue(d(2026, 11, 19, 15, 0))).toBe('19 Kasım 2026, 15:00')
    expect(longValue(d(2027, 1, 5, 9, 7))).toBe('5 Ocak 2027, 09:07')
  })
})
