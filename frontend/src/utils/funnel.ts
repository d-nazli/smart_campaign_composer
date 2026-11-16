import type { Audience } from '../types'

export interface Funnel {
  matched: number
  sms: number
  email: number
  /** Reachable on both channels. */
  both: number
  smsOnly: number
  emailOnly: number
  /** Matched people neither channel can reach. */
  unreachable: number
}

/** Splits the matched people by which channels reach them. All numbers come from the audience count. */
export function funnel(a: Audience): Funnel {
  const smsOnly = a.sms_reachable - a.both_reachable
  const emailOnly = a.email_reachable - a.both_reachable
  const unreachable = Math.max(0, a.matched - (smsOnly + emailOnly + a.both_reachable))
  return { matched: a.matched, sms: a.sms_reachable, email: a.email_reachable, both: a.both_reachable, smsOnly, emailOnly, unreachable }
}

/** Width of a funnel bar as a share of the matched people (0-100, at least a sliver for a non-zero count). */
export function share(n: number, matched: number): number {
  if (matched <= 0 || n <= 0) return 0
  return Math.max(2, Math.min(100, (n / matched) * 100))
}
