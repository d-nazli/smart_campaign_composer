// Date arithmetic behind the date-and-time picker. Plain functions, no browser: the picker
// only draws what these decide. All dates are in the viewer's own time zone.

export const WEEKDAYS = ['Pzt', 'Sal', 'Çar', 'Per', 'Cum', 'Cmt', 'Paz']
export const MONTHS = ['Ocak', 'Şubat', 'Mart', 'Nisan', 'Mayıs', 'Haziran', 'Temmuz', 'Ağustos', 'Eylül', 'Ekim', 'Kasım', 'Aralık']

export interface Cell {
  date: Date
  /** The day belongs to the month on screen (not to its neighbours shown to fill the weeks). */
  inMonth: boolean
}

export const startOfDay = (d: Date): Date => new Date(d.getFullYear(), d.getMonth(), d.getDate())
export const endOfDay = (d: Date): Date => new Date(d.getFullYear(), d.getMonth(), d.getDate(), 23, 59, 59, 999)

export const sameDay = (a: Date, b: Date): boolean =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()

/** Six weeks starting on Monday that contain the whole month (month is 0-11). Always 42 cells, so the popup keeps its height. */
export function monthGrid(year: number, month: number): Cell[] {
  const first = new Date(year, month, 1)
  const offset = (first.getDay() + 6) % 7 // Monday = 0
  const cells: Cell[] = []
  for (let i = 0; i < 42; i++) {
    const date = new Date(year, month, 1 - offset + i)
    cells.push({ date, inMonth: date.getMonth() === month })
  }
  return cells
}

export const monthLabel = (year: number, month: number): string => `${MONTHS[month]} ${year}`

/** A whole day cannot be chosen: it ends before `min` or starts after `max`. */
export function dayDisabled(day: Date, min: Date | null, max: Date | null): boolean {
  if (min && endOfDay(day).getTime() < min.getTime()) return true
  if (max && startOfDay(day).getTime() > max.getTime()) return true
  return false
}

/** `d` moved into [min, max] (either end may be missing). */
export function clampDate(d: Date, min: Date | null, max: Date | null): Date {
  if (min && d.getTime() < min.getTime()) return new Date(min)
  if (max && d.getTime() > max.getTime()) return new Date(max)
  return d
}

/** The chosen day with the time kept (09:00 when none was chosen yet), moved into range if needed. */
export function withDay(current: Date | null, day: Date, min: Date | null, max: Date | null): Date {
  const h = current ? current.getHours() : 9
  const m = current ? current.getMinutes() : 0
  return clampDate(new Date(day.getFullYear(), day.getMonth(), day.getDate(), h, m), min, max)
}

/** The current day (or `fallbackDay`) with a new time, moved into range if needed. */
export function withTime(current: Date | null, fallbackDay: Date, hour: number, minute: number, min: Date | null, max: Date | null): Date {
  const day = current ?? fallbackDay
  return clampDate(new Date(day.getFullYear(), day.getMonth(), day.getDate(), hour, minute), min, max)
}

/** A time slot cannot be chosen: that exact moment is outside [min, max]. */
export function momentDisabled(at: Date, min: Date | null, max: Date | null): boolean {
  return (min !== null && at.getTime() < min.getTime()) || (max !== null && at.getTime() > max.getTime())
}

const two = (n: number) => String(n).padStart(2, '0')

/** "16.11.2026 14:30" */
export const displayValue = (d: Date): string => `${two(d.getDate())}.${two(d.getMonth() + 1)}.${d.getFullYear()} ${two(d.getHours())}:${two(d.getMinutes())}`

/** "19 Kasım 2026, 15:00": the chosen moment written out, inside the picker. */
export const longValue = (d: Date): string => `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}, ${two(d.getHours())}:${two(d.getMinutes())}`

/** Hours offered in the time column: 00–23. */
export const HOURS: readonly number[] = Array.from({ length: 24 }, (_, i) => i)

/** Minutes offered in the time column: every minute, 00–59. */
export const MINUTES: readonly number[] = Array.from({ length: 60 }, (_, i) => i)

/** The month before/after (year and month, month 0-11). */
export function shiftMonth(year: number, month: number, by: number): { year: number; month: number } {
  const d = new Date(year, month + by, 1)
  return { year: d.getFullYear(), month: d.getMonth() }
}
