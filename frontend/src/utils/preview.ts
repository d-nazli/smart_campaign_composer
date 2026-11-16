import type { MessageContent } from '../types'

/** Sample recipient used in the previews, so {{first_name}} reads like a real message. */
export const SAMPLE: Record<string, string> = { first_name: 'Elif', city: 'İstanbul' }

/** Replaces known {{variables}} with the sample values; unknown ones are left as typed. */
export function fillSample(text: string, sample: Record<string, string> = SAMPLE): string {
  return text.replace(/\{\{\s*([a-z_]+)\s*\}\}/g, (whole, name: string) => sample[name] ?? whole)
}

/** The field the text is being written into: the last one that has any text (SMS before anything arrives). */
export function activeField(draft: MessageContent): keyof MessageContent {
  if (draft.email_body) return 'email_body'
  if (draft.email_subject) return 'email_subject'
  return 'sms_body'
}

export type MeterLevel = 'ok' | 'near' | 'over'

/** Green until 90% of the limit, yellow up to the limit, red beyond it. */
export function meterLevel(used: number, max: number): MeterLevel {
  if (used > max) return 'over'
  return used >= max * 0.9 ? 'near' : 'ok'
}

/** Badge text while a generation runs: the first try "üretiliyor", later tries "düzeltiliyor 2/3". */
export function attemptLabel(attempt: { n: number; max: number } | null): string {
  if (!attempt) return 'üretiliyor'
  return attempt.n > 1 ? `düzeltiliyor ${attempt.n}/${attempt.max}` : `üretiliyor ${attempt.n}/${attempt.max}`
}
